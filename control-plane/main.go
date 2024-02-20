package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"sync"
	"time"

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
)

const (
	splitter = "/"
	httpPort = 7001
)

type RequestBufferController struct {
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

	controller, err := newRequestBufferController(k8sInformerFactory, gwInformerFactory)
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
	srv := &http.Server{
		Addr:         ":" + strconv.Itoa(httpPort),
		WriteTimeout: 5 * time.Second,
		ReadTimeout:  5 * time.Second,
	}
	log.Printf("Starting HTTP server on port: %d\n", httpPort)
	log.Fatal(srv.ListenAndServe())
}

func newRequestBufferController(k8sInformerFactory informers.SharedInformerFactory, gwInformerFactory gwinformers.SharedInformerFactory) (*RequestBufferController, error) {
	endpointsInformer := k8sInformerFactory.Core().V1().Endpoints()
	httpRouteInformer := gwInformerFactory.Gateway().V1().HTTPRoutes()

	c := &RequestBufferController{
		k8sInformerFactory:  k8sInformerFactory,
		gwInformerFactory:   gwInformerFactory,
		endpointsInformer:   endpointsInformer,
		httpRouteInformer:   httpRouteInformer,
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
