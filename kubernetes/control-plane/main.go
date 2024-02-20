package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	autoscalingv1 "k8s.io/api/autoscaling/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	gwapi "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned"
	gwinformers "sigs.k8s.io/gateway-api/pkg/client/informers/externalversions"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
	"sigs.k8s.io/gateway-api/pkg/client/informers/externalversions/apis/v1"

	coreinformers "k8s.io/client-go/informers/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	splitter = "/"
	httpPort = 7001
)

type RequestBufferController struct {
	k8sClient *kubernetes.Clientset

	k8sInformerFactory informers.SharedInformerFactory
	gwInformerFactory  gwinformers.SharedInformerFactory

	endpointsInformer coreinformers.EndpointsInformer
	httpRouteInformer v1.HTTPRouteInformer

	mux                 sync.RWMutex
	scaledToZeroTargets map[string][]string // [service-name][]domains
}

func main() {
	log.Println("Starting kubernetes watchers")
	config, err := rest.InClusterConfig()
	if err != nil {
		log.Println("No in-cluster config found, trying with kubeconfig")
		var kubeconfig *string
		if home := homedir.HomeDir(); home != "" {
			kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
		} else {
			kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
		}
		flag.Parse()
		config, err = clientcmd.BuildConfigFromFlags("", *kubeconfig)
		if err != nil {
			log.Fatalf("Failed to create K8s client: %v", err)
		}
	}
	k8sClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("Failed to create K8s client: %v", err)
	}

	gwClient, err := gwapi.NewForConfig(config)
	if err != nil {
		log.Fatalf("Failed to create GW-API client: %v", err)
	}

	k8sInformerFactory := informers.NewSharedInformerFactory(k8sClient, time.Hour*24)
	gwInformerFactory := gwinformers.NewSharedInformerFactory(gwClient, time.Hour*24)

	controller, err := newRequestBufferController(k8sClient, k8sInformerFactory, gwInformerFactory)
	if err != nil {
		log.Fatalf("Error creating controller: %v", err)
	}

	stop := make(chan struct{})
	defer close(stop)
	err = controller.Run(stop)
	if err != nil {
		log.Fatalf("Error running controller: %v", err)
	}

	// HTTP server to return the state to envoy
	http.HandleFunc("/", controller.getScaledToZeroClusters)
	http.HandleFunc("/poke-scale-up", controller.pokeScaleUp)
	srv := &http.Server{
		Addr:         ":" + strconv.Itoa(httpPort),
		WriteTimeout: 5 * time.Second,
		ReadTimeout:  5 * time.Second,
	}
	log.Printf("Starting HTTP server on port: %d", httpPort)
	log.Fatal(srv.ListenAndServe())
}

func newRequestBufferController(k8sClient *kubernetes.Clientset, k8sInformerFactory informers.SharedInformerFactory, gwInformerFactory gwinformers.SharedInformerFactory) (*RequestBufferController, error) {
	endpointsInformer := k8sInformerFactory.Core().V1().Endpoints()
	httpRouteInformer := gwInformerFactory.Gateway().V1().HTTPRoutes()

	c := &RequestBufferController{
		k8sClient: k8sClient,

		k8sInformerFactory: k8sInformerFactory,
		gwInformerFactory:  gwInformerFactory,

		endpointsInformer: endpointsInformer,
		httpRouteInformer: httpRouteInformer,

		scaledToZeroTargets: make(map[string][]string),
	}
	_, err := endpointsInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    c.endpointAdd,
			UpdateFunc: c.endpointUpdate,
			DeleteFunc: c.endpointDelete,
		},
	)
	if err != nil {
		return nil, err
	}
	_, err = httpRouteInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    c.routeAdd,
			UpdateFunc: c.routeUpdate,
			DeleteFunc: c.routeDelete,
		},
	)
	if err != nil {
		return nil, err
	}

	return c, nil
}

func (c *RequestBufferController) getScaledToZeroClusters(w http.ResponseWriter, r *http.Request) {
	c.mux.RLock()
	defer c.mux.RUnlock()

	domains := make([]string, 0, len(c.scaledToZeroTargets))
	for _, hosts := range c.scaledToZeroTargets {
		domains = append(domains, hosts...)
	}
	jsonStr, err := json.Marshal(domains)
	if err != nil {
		log.Println("failed to marshal scaledToZeroTargets, err: ", err)
		w.WriteHeader(http.StatusInternalServerError)
	}
	_, err = w.Write(jsonStr)
	if err != nil {
		log.Printf("failed to write to output stream, err: %v\n", err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func (c *RequestBufferController) pokeScaleUp(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	err := r.ParseForm()
	if err != nil {
		log.Printf("failed to POST parse form: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	hostname := r.Form.Get("host")

	if hostname == "" {
		log.Println("host not specified")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	routes, err := c.httpRouteInformer.Lister().List(labels.Everything())
	if err != nil {
		log.Printf("failed to list HTTPRoutes: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	for _, rt := range routes {
		for _, h := range rt.Spec.Hostnames {
			if string(h) == hostname {
				if err = c.triggerScaleUp(rt); err != nil {
					log.Printf("Failed to trigger scale-up: %v", err)
					w.WriteHeader(http.StatusInternalServerError)
				} else {
					w.WriteHeader(http.StatusOK)
				}
				return
			}
		}
	}

	log.Printf("Host :%s was not found in any HTTPRoute", hostname)
	w.WriteHeader(http.StatusNotFound)
}

func (c *RequestBufferController) triggerScaleUp(rt *gwapiv1.HTTPRoute) error {
	log.Printf("Triggering scale-up for HTTPRoute: %s/%s", rt.Namespace, rt.Name)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, rule := range rt.Spec.Rules {
		for _, ref := range rule.BackendRefs {
			if *ref.Kind == "Service" {
				// Get the Service labels
				service, err := c.k8sClient.CoreV1().Services(rt.Namespace).Get(ctx, string(ref.Name), metav1.GetOptions{})
				if err != nil {
					return err
				}

				// Get the deployment pointing to the service
				deployments, err := c.k8sClient.AppsV1().Deployments(rt.Namespace).List(ctx, metav1.ListOptions{})
				if err != nil {
					return err
				}
				if deployments == nil || len(deployments.Items) == 0 {
					return fmt.Errorf("could not find any deployments in namespace: %s", rt.Namespace)
				}

				for _, d := range deployments.Items {
					// Scale deployments where the Service selector matches the Deployment selectors
					for k, v := range service.Spec.Selector {
						if d.Spec.Selector.MatchLabels[k] == v {
							log.Printf("Scaling up deployment: %s/%s to replica=1", d.Namespace, d.Name)
							_, err = c.k8sClient.AppsV1().Deployments(rt.Namespace).UpdateScale(ctx, d.Name, &autoscalingv1.Scale{
								ObjectMeta: metav1.ObjectMeta{
									Name:      d.Name,
									Namespace: d.Namespace,
								},
								Spec: autoscalingv1.ScaleSpec{
									Replicas: 1,
								},
								Status: autoscalingv1.ScaleStatus{},
							}, metav1.UpdateOptions{})
							if err != nil {
								return err
							}
						}
					}
				}
			}
		}
	}
	return nil
}

func (c *RequestBufferController) Run(stopCh chan struct{}) error {
	c.k8sInformerFactory.Start(stopCh)
	c.gwInformerFactory.Start(stopCh)
	// wait for the initial synchronization of the local cache.
	if !cache.WaitForCacheSync(stopCh, c.endpointsInformer.Informer().HasSynced) {
		return fmt.Errorf("failed to sync K8s informers")
	}
	if !cache.WaitForCacheSync(stopCh, c.httpRouteInformer.Informer().HasSynced) {
		return fmt.Errorf("failed to sync GW-API informers")
	}
	return nil
}

func (c *RequestBufferController) handleRouteChange(route *gwapiv1.HTTPRoute) {
	isReady := c.hasReadyEndpoints(route)
	key := route.Namespace + splitter + route.Name

	log.Printf("HTTPRoute %s is considered ready: %v\n", key, isReady)

	c.mux.Lock()
	defer c.mux.Unlock()

	_, has := c.scaledToZeroTargets[key]

	switch {
	case !has && isReady:
		// noop, is not scaled to zero
	case has && isReady:
		// is no longer scaled to zero, need to remove it from the list
		delete(c.scaledToZeroTargets, key)
	case !isReady:
		// is scaled to zero, need to add/or update it to our list (domains might have changed)
		var domains []string
		for _, h := range route.Spec.Hostnames {
			domains = append(domains, string(h))
		}
		c.scaledToZeroTargets[key] = domains
	}
}

func (c *RequestBufferController) routeAdd(obj interface{}) {
	route, ok := obj.(*gwapiv1.HTTPRoute)
	if !ok {
		log.Printf("object is not a HTTPRoute: %v", obj)
		return
	}
	c.handleRouteChange(route)
}

func (c *RequestBufferController) routeUpdate(_, new interface{}) {
	route, ok := new.(*gwapiv1.HTTPRoute)
	if !ok {
		log.Printf("object is not a HTTPRoute: %v", new)
		return
	}
	c.handleRouteChange(route)
}

func (c *RequestBufferController) routeDelete(obj interface{}) {
	route, ok := obj.(*gwapiv1.HTTPRoute)
	if !ok {
		log.Printf("object is not a HTTP route: %v", obj)
		return
	}

	key := route.Namespace + splitter + route.Name

	c.mux.Lock()
	defer c.mux.Unlock()

	log.Printf("HTTPRoute %s was deleted, removing from scaledToZeroTargets", key)
	delete(c.scaledToZeroTargets, key)
}

func (c *RequestBufferController) handleEndpointChange(endpoint *corev1.Endpoints) {
	routes, err := c.gwInformerFactory.Gateway().V1().HTTPRoutes().Lister().HTTPRoutes(endpoint.Namespace).List(labels.Everything())
	if err != nil {
		log.Printf("Failed to list HTTPRoutes in namespace: %s, %v", endpoint.Namespace, err)
		// todo: better error management, fine for PoC
		return
	}

	for _, rt := range routes {
		for _, rule := range rt.Spec.Rules {
			for _, ref := range rule.BackendRefs {
				if *ref.Kind == "Service" && string(ref.Name) == endpoint.Name {
					c.handleRouteChange(rt)
				}
			}
		}
	}
}

func (c *RequestBufferController) endpointAdd(obj interface{}) {
	ep, ok := obj.(*corev1.Endpoints)
	if !ok {
		log.Printf("object is not an Endpoint: %v", obj)
		return
	}
	c.handleEndpointChange(ep)
}

func (c *RequestBufferController) endpointUpdate(_ interface{}, new interface{}) {
	ep, ok := new.(*corev1.Endpoints)
	if !ok {
		log.Printf("object is not an Endpoint: %v", new)
		return
	}
	c.handleEndpointChange(ep)

}

func (c *RequestBufferController) endpointDelete(obj interface{}) {
	ep, ok := obj.(*corev1.Endpoints)
	if !ok {
		log.Printf("object is not an Endpoint: %v", obj)
		return
	}
	c.handleEndpointChange(ep)
}

func (c *RequestBufferController) hasReadyEndpoints(route *gwapiv1.HTTPRoute) bool {
	readyEndpoints := make(map[string]bool)
	endpoints, err := c.endpointsInformer.Lister().Endpoints(route.Namespace).List(labels.Everything())
	if err != nil {
		log.Printf("Failed to list endpoints in namespace: %s, %v", route.Namespace, err)
		// todo: better error management, fine for PoC
		return false
	}
	for _, ep := range endpoints {
		for _, sub := range ep.Subsets {
			// if we have at least one ready address we consider the endpoint ready
			if len(sub.Addresses) > 0 {
				readyEndpoints[ep.Name] = true
				break
			}
		}
	}

	hasReadyEndpoints := true
	// Make sure every rule + backend ref with type "Service" is ready
	for _, r := range route.Spec.Rules {
		for _, b := range r.BackendRefs {
			// for now, we only handle "Service"
			if *b.Kind == "Service" {
				if _, has := readyEndpoints[string(b.Name)]; !has {
					hasReadyEndpoints = false
				}
			}
		}
	}

	return hasReadyEndpoints
}
