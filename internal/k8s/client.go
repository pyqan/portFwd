package k8s

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"

	"github.com/pyqan/portFwd/internal/logger"
)

// Client wraps Kubernetes client with helper methods
type Client struct {
	clientset  *kubernetes.Clientset
	restConfig *rest.Config
}

// PodInfo contains pod information for display
type PodInfo struct {
	Name      string
	Namespace string
	Status    string
	Ports     []ContainerPort
}

// ContainerPort represents a container port
type ContainerPort struct {
	Name          string
	ContainerPort int32
	Protocol      string
}

// ServiceInfo contains service information for display
type ServiceInfo struct {
	Name      string
	Namespace string
	Type      string
	Ports     []ServicePort
}

// ServicePort represents a service port
type ServicePort struct {
	Name       string
	Port       int32
	TargetPort string
	Protocol   string
}

// NewClient creates a new Kubernetes client
// Based on: https://github.com/kubernetes/client-go/tree/master/examples/out-of-cluster-client-configuration
func NewClient() (*Client, error) {
	logger.Debug("k8s", "Creating new Kubernetes client...")
	config, err := getKubeConfig()
	if err != nil {
		logger.Error("k8s", "Failed to get kubeconfig: %v", err)
		return nil, fmt.Errorf("failed to get kubeconfig: %w", err)
	}
	logger.Debug("k8s", "Kubeconfig loaded, API server: %s", config.Host)

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		logger.Error("k8s", "Failed to create clientset: %v", err)
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}
	logger.Info("k8s", "Kubernetes client created successfully")

	return &Client{
		clientset:  clientset,
		restConfig: config,
	}, nil
}

// NewClientWithKubeconfig creates a client with specific kubeconfig path
func NewClientWithKubeconfig(kubeconfigPath string) (*Client, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to build config from %s: %w", kubeconfigPath, err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}

	return &Client{
		clientset:  clientset,
		restConfig: config,
	}, nil
}

// getKubeConfig returns the Kubernetes configuration
// Based on: https://github.com/kubernetes/client-go/tree/master/examples/out-of-cluster-client-configuration

func getKubeConfig() (*rest.Config, error) {
	// Try in-cluster config first (when running inside a pod)
	config, err := rest.InClusterConfig()
	if err == nil {
		return config, nil
	}

	// Fall back to kubeconfig file (out-of-cluster)
	var kubeconfig string

	// Check KUBECONFIG env var first
	if envKubeconfig := os.Getenv("KUBECONFIG"); envKubeconfig != "" {
		kubeconfig = envKubeconfig
	} else if home := homedir.HomeDir(); home != "" {
		// Use default location ~/.kube/config
		kubeconfig = filepath.Join(home, ".kube", "config")
	} else {
		return nil, fmt.Errorf("unable to locate kubeconfig file")
	}

	// Use the current context in kubeconfig
	config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to build config from kubeconfig: %w", err)
	}

	return config, nil
}

// GetRestConfig returns the REST config for port-forwarding
func (c *Client) GetRestConfig() *rest.Config {
	return c.restConfig
}

// GetClientset returns the Kubernetes clientset
func (c *Client) GetClientset() *kubernetes.Clientset {
	return c.clientset
}

// GetNamespaces returns list of all namespaces
func (c *Client) GetNamespaces(ctx context.Context) ([]string, error) {
	logger.Debug("k8s", "Listing namespaces...")
	namespaces, err := c.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		logger.Error("k8s", "Failed to list namespaces: %v", err)
		return nil, fmt.Errorf("failed to list namespaces: %w", err)
	}

	result := make([]string, 0, len(namespaces.Items))
	for _, ns := range namespaces.Items {
		result = append(result, ns.Name)
	}
	sort.Strings(result)
	logger.Debug("k8s", "Found %d namespaces", len(result))
	return result, nil
}

// GetPods returns list of pods in a namespace
func (c *Client) GetPods(ctx context.Context, namespace string) ([]PodInfo, error) {
	logger.Debug("k8s", "Listing pods in namespace: %s", namespace)
	pods, err := c.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		logger.Error("k8s", "Failed to list pods in %s: %v", namespace, err)
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	result := make([]PodInfo, 0, len(pods.Items))
	for _, pod := range pods.Items {
		podInfo := PodInfo{
			Name:      pod.Name,
			Namespace: pod.Namespace,
			Status:    string(pod.Status.Phase),
			Ports:     make([]ContainerPort, 0),
		}

		for _, container := range pod.Spec.Containers {
			for _, port := range container.Ports {
				podInfo.Ports = append(podInfo.Ports, ContainerPort{
					Name:          port.Name,
					ContainerPort: port.ContainerPort,
					Protocol:      string(port.Protocol),
				})
			}
		}

		result = append(result, podInfo)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	logger.Debug("k8s", "Found %d pods in namespace %s", len(result), namespace)
	return result, nil
}

// GetRunningPods returns list of running pods in a namespace
func (c *Client) GetRunningPods(ctx context.Context, namespace string) ([]PodInfo, error) {
	pods, err := c.GetPods(ctx, namespace)
	if err != nil {
		return nil, err
	}

	result := make([]PodInfo, 0)
	for _, pod := range pods {
		if pod.Status == string(corev1.PodRunning) {
			result = append(result, pod)
		}
	}
	return result, nil
}

// GetServices returns list of services in a namespace
func (c *Client) GetServices(ctx context.Context, namespace string) ([]ServiceInfo, error) {
	logger.Debug("k8s", "Listing services in namespace: %s", namespace)
	services, err := c.clientset.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		logger.Error("k8s", "Failed to list services in %s: %v", namespace, err)
		return nil, fmt.Errorf("failed to list services: %w", err)
	}

	result := make([]ServiceInfo, 0, len(services.Items))
	for _, svc := range services.Items {
		svcInfo := ServiceInfo{
			Name:      svc.Name,
			Namespace: svc.Namespace,
			Type:      string(svc.Spec.Type),
			Ports:     make([]ServicePort, 0),
		}

		for _, port := range svc.Spec.Ports {
			svcInfo.Ports = append(svcInfo.Ports, ServicePort{
				Name:       port.Name,
				Port:       port.Port,
				TargetPort: port.TargetPort.String(),
				Protocol:   string(port.Protocol),
			})
		}

		result = append(result, svcInfo)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	logger.Debug("k8s", "Found %d services in namespace %s", len(result), namespace)
	return result, nil
}

// GetPod returns a specific pod
// GetService returns a single service by name
func (c *Client) GetService(ctx context.Context, namespace, name string) (*ServiceInfo, error) {
	svc, err := c.clientset.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get service: %w", err)
	}

	svcInfo := &ServiceInfo{
		Name:      svc.Name,
		Namespace: svc.Namespace,
		Type:      string(svc.Spec.Type),
		Ports:     make([]ServicePort, 0),
	}

	for _, port := range svc.Spec.Ports {
		svcInfo.Ports = append(svcInfo.Ports, ServicePort{
			Name:       port.Name,
			Port:       port.Port,
			TargetPort: port.TargetPort.String(),
			Protocol:   string(port.Protocol),
		})
	}

	return svcInfo, nil
}

func (c *Client) GetPod(ctx context.Context, namespace, name string) (*PodInfo, error) {
	pod, err := c.clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get pod: %w", err)
	}

	podInfo := &PodInfo{
		Name:      pod.Name,
		Namespace: pod.Namespace,
		Status:    string(pod.Status.Phase),
		Ports:     make([]ContainerPort, 0),
	}

	for _, container := range pod.Spec.Containers {
		for _, port := range container.Ports {
			podInfo.Ports = append(podInfo.Ports, ContainerPort{
				Name:          port.Name,
				ContainerPort: port.ContainerPort,
				Protocol:      string(port.Protocol),
			})
		}
	}

	return podInfo, nil
}

// ServiceTargetInfo contains pod and port information for a service
type ServiceTargetInfo struct {
	PodName    string
	Namespace  string
	TargetPort int
}

// GetPodForService finds a running pod that backs the given service
func (c *Client) GetPodForService(ctx context.Context, namespace, serviceName string) (*PodInfo, error) {
	info, err := c.GetServiceTarget(ctx, namespace, serviceName, 0)
	if err != nil {
		return nil, err
	}
	return c.GetPod(ctx, namespace, info.PodName)
}

// GetServiceTarget finds a running pod and resolves targetPort for a service
// If servicePort is 0, uses the first port defined in the service
func (c *Client) GetServiceTarget(ctx context.Context, namespace, serviceName string, servicePort int) (*ServiceTargetInfo, error) {
	logger.Debug("k8s", "GetServiceTarget: %s/%s port=%d", namespace, serviceName, servicePort)
	
	// Get the service to find its selector and ports
	svc, err := c.clientset.CoreV1().Services(namespace).Get(ctx, serviceName, metav1.GetOptions{})
	if err != nil {
		logger.Error("k8s", "GetServiceTarget: failed to get service %s/%s: %v", namespace, serviceName, err)
		return nil, fmt.Errorf("failed to get service: %w", err)
	}
	logger.Debug("k8s", "GetServiceTarget: service found, type=%s, clusterIP=%s", svc.Spec.Type, svc.Spec.ClusterIP)

	if len(svc.Spec.Selector) == 0 {
		logger.Error("k8s", "GetServiceTarget: service %s has no selector", serviceName)
		return nil, fmt.Errorf("service %s has no selector", serviceName)
	}

	// Find the target port for the given service port
	var targetPort int
	for _, port := range svc.Spec.Ports {
		logger.Debug("k8s", "GetServiceTarget: checking port %d -> %v", port.Port, port.TargetPort)
		if servicePort == 0 || int(port.Port) == servicePort {
			// targetPort can be a number or a named port
			if port.TargetPort.IntValue() != 0 {
				targetPort = port.TargetPort.IntValue()
				logger.Debug("k8s", "GetServiceTarget: targetPort (numeric) = %d", targetPort)
			} else {
				// Named port - need to resolve from pod
				targetPort = int(port.Port) // fallback to service port
				logger.Debug("k8s", "GetServiceTarget: targetPort (fallback) = %d", targetPort)
			}
			break
		}
	}

	if targetPort == 0 {
		logger.Error("k8s", "GetServiceTarget: port %d not found in service %s", servicePort, serviceName)
		return nil, fmt.Errorf("port %d not found in service %s", servicePort, serviceName)
	}

	// Build label selector from service selector
	var selectorParts []string
	for k, v := range svc.Spec.Selector {
		selectorParts = append(selectorParts, fmt.Sprintf("%s=%s", k, v))
	}
	labelSelector := ""
	for i, part := range selectorParts {
		if i > 0 {
			labelSelector += ","
		}
		labelSelector += part
	}
	logger.Debug("k8s", "GetServiceTarget: label selector = %s", labelSelector)

	// List pods matching the selector
	pods, err := c.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		logger.Error("k8s", "GetServiceTarget: failed to list pods: %v", err)
		return nil, fmt.Errorf("failed to list pods for service: %w", err)
	}
	logger.Debug("k8s", "GetServiceTarget: found %d pods matching selector", len(pods.Items))

	// Find a running pod
	for _, pod := range pods.Items {
		logger.Debug("k8s", "GetServiceTarget: pod %s phase=%s", pod.Name, pod.Status.Phase)
		if pod.Status.Phase == corev1.PodRunning {
			logger.Info("k8s", "GetServiceTarget: selected pod %s, targetPort=%d", pod.Name, targetPort)
			return &ServiceTargetInfo{
				PodName:    pod.Name,
				Namespace:  pod.Namespace,
				TargetPort: targetPort,
			}, nil
		}
	}

	logger.Error("k8s", "GetServiceTarget: no running pods found for service %s", serviceName)
	return nil, fmt.Errorf("no running pods found for service %s", serviceName)
}

// GetCurrentContext returns the current Kubernetes context name
func (c *Client) GetCurrentContext() (string, error) {
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		kubeconfig = filepath.Join(home, ".kube", "config")
	}

	config, err := clientcmd.LoadFromFile(kubeconfig)
	if err != nil {
		return "", err
	}

	return config.CurrentContext, nil
}
