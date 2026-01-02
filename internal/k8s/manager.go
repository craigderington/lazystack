package k8s

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsclientset "k8s.io/metrics/pkg/client/clientset/versioned"
	"sigs.k8s.io/yaml"
)

var _ = appsv1.Deployment{}         // Force import usage
var _ = metricsv1beta1.PodMetrics{} // Force import usage

// PodInfo represents a Kubernetes pod
type PodInfo struct {
	Name      string
	Namespace string
	Status    string
	Ready     string
	Restarts  int32
	Age       string
}

// DeploymentInfo represents a Kubernetes deployment
type DeploymentInfo struct {
	Name      string
	Namespace string
	Ready     string
	UpToDate  int32
	Available int32
	Replicas  int32
}

// ServiceInfo represents a Kubernetes service
type ServiceInfo struct {
	Name         string
	Namespace    string
	Type         string // ClusterIP, NodePort, LoadBalancer, ExternalName
	ClusterIP    string
	ExternalIP   string
	Ports        string
}

// PodMetrics represents resource usage metrics for a pod
type PodMetrics struct {
	Name              string
	Namespace         string
	CPUUsageMillis    int64  // CPU usage in millicores
	MemoryUsageBytes  int64  // Memory usage in bytes
	CPUUsageFormatted string // e.g., "250m"
	MemoryUsageFormatted string // e.g., "512Mi"
}

// EnvVar represents an environment variable
type EnvVar struct {
	Name       string
	Value      string
	ValueFrom  string // e.g., "ConfigMap: config-name", "Secret: secret-name"
}

// PodEnvVars represents all environment variables for a pod
type PodEnvVars struct {
	PodName    string
	Namespace  string
	Containers map[string][]EnvVar // container name -> env vars
}

// PortForward represents an active port forward
type PortForward struct {
	PodName    string
	LocalPort  string
	RemotePort string
	Cmd        *exec.Cmd
}

// Manager handles Kubernetes interactions
type Manager struct {
	clientset     *kubernetes.Clientset
	metricsClient *metricsclientset.Clientset
	namespace     string
}

// NewManager creates a new Kubernetes manager
func NewManager() (*Manager, error) {
	var kubeconfig string

	// 1. Check KUBECONFIG environment variable
	if envKubeconfig := os.Getenv("KUBECONFIG"); envKubeconfig != "" {
		kubeconfig = envKubeconfig
	} else {
		// 2. If running as sudo, try to get the actual user's home directory
		homeDir := homedir.HomeDir()
		if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" && homeDir == "/root" {
			// We're running with sudo, use the actual user's home
			homeDir = filepath.Join("/home", sudoUser)
		}
		kubeconfig = filepath.Join(homeDir, ".kube", "config")
	}

	// Build config from kubeconfig file
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to build config from %s: %w", kubeconfig, err)
	}

	// Create clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}

	// Create metrics clientset (may fail if metrics-server not installed)
	metricsClient, err := metricsclientset.NewForConfig(config)
	if err != nil {
		// Don't fail if metrics client can't be created - metrics just won't be available
		metricsClient = nil
	}

	return &Manager{
		clientset:     clientset,
		metricsClient: metricsClient,
		namespace:     "default",
	}, nil
}

// SetNamespace sets the current namespace
func (m *Manager) SetNamespace(namespace string) {
	m.namespace = namespace
}

// GetNamespace returns the current namespace
func (m *Manager) GetNamespace() string {
	return m.namespace
}

// ListNamespaces returns a list of all namespaces
func (m *Manager) ListNamespaces() ([]string, error) {
	namespaces, err := m.clientset.CoreV1().Namespaces().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces: %w", err)
	}

	names := make([]string, 0, len(namespaces.Items))
	for _, ns := range namespaces.Items {
		names = append(names, ns.Name)
	}

	return names, nil
}

// ListPods returns a list of pods in the current namespace
func (m *Manager) ListPods() ([]PodInfo, error) {
	pods, err := m.clientset.CoreV1().Pods(m.namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	podInfos := make([]PodInfo, 0, len(pods.Items))
	for _, pod := range pods.Items {
		podInfo := PodInfo{
			Name:      pod.Name,
			Namespace: pod.Namespace,
			Status:    string(pod.Status.Phase),
			Ready:     m.getPodReadyStatus(&pod),
			Restarts:  m.getPodRestarts(&pod),
		}
		podInfos = append(podInfos, podInfo)
	}

	return podInfos, nil
}

// DeletePod deletes a pod by name
func (m *Manager) DeletePod(name string) error {
	err := m.clientset.CoreV1().Pods(m.namespace).Delete(
		context.Background(),
		name,
		metav1.DeleteOptions{},
	)
	if err != nil {
		return fmt.Errorf("failed to delete pod %s: %w", name, err)
	}
	return nil
}

// GetPodLogs returns logs for a specific pod
func (m *Manager) GetPodLogs(name string, lines int64) (string, error) {
	podLogOpts := corev1.PodLogOptions{
		TailLines: &lines,
	}

	req := m.clientset.CoreV1().Pods(m.namespace).GetLogs(name, &podLogOpts)
	logs, err := req.DoRaw(context.Background())
	if err != nil {
		return "", fmt.Errorf("failed to get logs for pod %s: %w", name, err)
	}

	return string(logs), nil
}

// getPodReadyStatus returns a string representing the ready status (e.g., "2/3")
func (m *Manager) getPodReadyStatus(pod *corev1.Pod) string {
	totalContainers := len(pod.Status.ContainerStatuses)
	readyContainers := 0

	for _, status := range pod.Status.ContainerStatuses {
		if status.Ready {
			readyContainers++
		}
	}

	return fmt.Sprintf("%d/%d", readyContainers, totalContainers)
}

// getPodRestarts returns the total number of container restarts in a pod
func (m *Manager) getPodRestarts(pod *corev1.Pod) int32 {
	var restarts int32
	for _, status := range pod.Status.ContainerStatuses {
		restarts += status.RestartCount
	}
	return restarts
}

// ListDeployments returns a list of deployments in the current namespace
func (m *Manager) ListDeployments() ([]DeploymentInfo, error) {
	deployments, err := m.clientset.AppsV1().Deployments(m.namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list deployments: %w", err)
	}

	deploymentInfos := make([]DeploymentInfo, 0, len(deployments.Items))
	for _, deploy := range deployments.Items {
		ready := fmt.Sprintf("%d/%d", deploy.Status.ReadyReplicas, deploy.Status.Replicas)
		deploymentInfo := DeploymentInfo{
			Name:      deploy.Name,
			Namespace: deploy.Namespace,
			Ready:     ready,
			UpToDate:  deploy.Status.UpdatedReplicas,
			Available: deploy.Status.AvailableReplicas,
			Replicas:  deploy.Status.Replicas,
		}
		deploymentInfos = append(deploymentInfos, deploymentInfo)
	}

	return deploymentInfos, nil
}

// ListServices returns a list of Kubernetes services in the current namespace
func (m *Manager) ListServices() ([]ServiceInfo, error) {
	services, err := m.clientset.CoreV1().Services(m.namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}

	serviceInfos := make([]ServiceInfo, 0, len(services.Items))
	for _, svc := range services.Items {
		// Build ports string
		ports := ""
		for i, port := range svc.Spec.Ports {
			if i > 0 {
				ports += ","
			}
			ports += fmt.Sprintf("%d/%s", port.Port, port.Protocol)
		}

		// Get external IP
		externalIP := "<none>"
		if len(svc.Status.LoadBalancer.Ingress) > 0 {
			if svc.Status.LoadBalancer.Ingress[0].IP != "" {
				externalIP = svc.Status.LoadBalancer.Ingress[0].IP
			} else if svc.Status.LoadBalancer.Ingress[0].Hostname != "" {
				externalIP = svc.Status.LoadBalancer.Ingress[0].Hostname
			}
		} else if len(svc.Spec.ExternalIPs) > 0 {
			externalIP = svc.Spec.ExternalIPs[0]
		}

		serviceInfo := ServiceInfo{
			Name:       svc.Name,
			Namespace:  svc.Namespace,
			Type:       string(svc.Spec.Type),
			ClusterIP:  svc.Spec.ClusterIP,
			ExternalIP: externalIP,
			Ports:      ports,
		}
		serviceInfos = append(serviceInfos, serviceInfo)
	}

	return serviceInfos, nil
}

// GetPodMetrics returns resource usage metrics for a specific pod
func (m *Manager) GetPodMetrics(podName string) (*PodMetrics, error) {
	if m.metricsClient == nil {
		return nil, fmt.Errorf("metrics-server not available")
	}

	podMetrics, err := m.metricsClient.MetricsV1beta1().PodMetricses(m.namespace).Get(context.Background(), podName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get metrics for pod %s: %w", podName, err)
	}

	// Aggregate metrics across all containers in the pod
	var totalCPU int64
	var totalMemory int64

	for _, container := range podMetrics.Containers {
		cpuQuantity := container.Usage.Cpu()
		memQuantity := container.Usage.Memory()

		totalCPU += cpuQuantity.MilliValue()
		totalMemory += memQuantity.Value()
	}

	// Format CPU (millicores to readable format)
	cpuFormatted := fmt.Sprintf("%dm", totalCPU)
	if totalCPU >= 1000 {
		cpuFormatted = fmt.Sprintf("%.2f", float64(totalCPU)/1000.0)
	}

	// Format Memory (bytes to Mi/Gi)
	memFormatted := fmt.Sprintf("%dMi", totalMemory/(1024*1024))
	if totalMemory >= 1024*1024*1024 {
		memFormatted = fmt.Sprintf("%.2fGi", float64(totalMemory)/(1024*1024*1024))
	}

	return &PodMetrics{
		Name:                 podName,
		Namespace:            m.namespace,
		CPUUsageMillis:       totalCPU,
		MemoryUsageBytes:     totalMemory,
		CPUUsageFormatted:    cpuFormatted,
		MemoryUsageFormatted: memFormatted,
	}, nil
}

// GetPodEnvVars returns all environment variables for a specific pod
func (m *Manager) GetPodEnvVars(podName string) (*PodEnvVars, error) {
	pod, err := m.clientset.CoreV1().Pods(m.namespace).Get(context.Background(), podName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get pod %s: %w", podName, err)
	}

	result := &PodEnvVars{
		PodName:    podName,
		Namespace:  m.namespace,
		Containers: make(map[string][]EnvVar),
	}

	// Iterate through all containers in the pod
	for _, container := range pod.Spec.Containers {
		envVars := []EnvVar{}

		for _, env := range container.Env {
			envVar := EnvVar{
				Name: env.Name,
			}

			// Check if value is directly specified
			if env.Value != "" {
				envVar.Value = env.Value
			} else if env.ValueFrom != nil {
				// Value comes from ConfigMap, Secret, FieldRef, or ResourceFieldRef
				if env.ValueFrom.ConfigMapKeyRef != nil {
					envVar.ValueFrom = fmt.Sprintf("ConfigMap: %s (key: %s)",
						env.ValueFrom.ConfigMapKeyRef.Name,
						env.ValueFrom.ConfigMapKeyRef.Key)
				} else if env.ValueFrom.SecretKeyRef != nil {
					envVar.ValueFrom = fmt.Sprintf("Secret: %s (key: %s)",
						env.ValueFrom.SecretKeyRef.Name,
						env.ValueFrom.SecretKeyRef.Key)
					envVar.Value = "<secret>"
				} else if env.ValueFrom.FieldRef != nil {
					envVar.ValueFrom = fmt.Sprintf("FieldRef: %s", env.ValueFrom.FieldRef.FieldPath)
				} else if env.ValueFrom.ResourceFieldRef != nil {
					envVar.ValueFrom = fmt.Sprintf("ResourceFieldRef: %s", env.ValueFrom.ResourceFieldRef.Resource)
				}
			}

			envVars = append(envVars, envVar)
		}

		// Also check for envFrom (ConfigMapRef, SecretRef)
		for _, envFrom := range container.EnvFrom {
			if envFrom.ConfigMapRef != nil {
				envVars = append(envVars, EnvVar{
					Name:      fmt.Sprintf("(All keys from ConfigMap: %s)", envFrom.ConfigMapRef.Name),
					ValueFrom: fmt.Sprintf("ConfigMap: %s", envFrom.ConfigMapRef.Name),
				})
			}
			if envFrom.SecretRef != nil {
				envVars = append(envVars, EnvVar{
					Name:      fmt.Sprintf("(All keys from Secret: %s)", envFrom.SecretRef.Name),
					ValueFrom: fmt.Sprintf("Secret: %s", envFrom.SecretRef.Name),
					Value:     "<secret>",
				})
			}
		}

		result.Containers[container.Name] = envVars
	}

	return result, nil
}

// GetPodYAML returns the YAML representation of a pod
func (m *Manager) GetPodYAML(podName string) (string, error) {
	pod, err := m.clientset.CoreV1().Pods(m.namespace).Get(context.Background(), podName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get pod %s: %w", podName, err)
	}

	// Convert to YAML
	yamlBytes, err := yaml.Marshal(pod)
	if err != nil {
		return "", fmt.Errorf("failed to marshal pod to YAML: %w", err)
	}

	return string(yamlBytes), nil
}

// GetDeploymentYAML returns the YAML representation of a deployment
func (m *Manager) GetDeploymentYAML(deploymentName string) (string, error) {
	deployment, err := m.clientset.AppsV1().Deployments(m.namespace).Get(context.Background(), deploymentName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get deployment %s: %w", deploymentName, err)
	}

	// Convert to YAML
	yamlBytes, err := yaml.Marshal(deployment)
	if err != nil {
		return "", fmt.Errorf("failed to marshal deployment to YAML: %w", err)
	}

	return string(yamlBytes), nil
}

// GetServiceYAML returns the YAML representation of a service
func (m *Manager) GetServiceYAML(serviceName string) (string, error) {
	service, err := m.clientset.CoreV1().Services(m.namespace).Get(context.Background(), serviceName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get service %s: %w", serviceName, err)
	}

	// Convert to YAML
	yamlBytes, err := yaml.Marshal(service)
	if err != nil {
		return "", fmt.Errorf("failed to marshal service to YAML: %w", err)
	}

	return string(yamlBytes), nil
}

// ScaleDeployment scales a deployment to the specified number of replicas
func (m *Manager) ScaleDeployment(name string, replicas int32) error {
	deployment, err := m.clientset.AppsV1().Deployments(m.namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get deployment %s: %w", name, err)
	}

	deployment.Spec.Replicas = &replicas

	_, err = m.clientset.AppsV1().Deployments(m.namespace).Update(context.Background(), deployment, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to scale deployment %s: %w", name, err)
	}

	return nil
}

// RestartDeployment restarts a deployment by updating its annotation
func (m *Manager) RestartDeployment(name string) error {
	deployment, err := m.clientset.AppsV1().Deployments(m.namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get deployment %s: %w", name, err)
	}

	// Update annotation to trigger rollout
	if deployment.Spec.Template.Annotations == nil {
		deployment.Spec.Template.Annotations = make(map[string]string)
	}
	deployment.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"] = time.Now().Format(time.RFC3339)

	_, err = m.clientset.AppsV1().Deployments(m.namespace).Update(context.Background(), deployment, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to restart deployment %s: %w", name, err)
	}

	return nil
}

// DeleteDeployment deletes a deployment by name
func (m *Manager) DeleteDeployment(name string) error {
	err := m.clientset.AppsV1().Deployments(m.namespace).Delete(
		context.Background(),
		name,
		metav1.DeleteOptions{},
	)
	if err != nil {
		return fmt.Errorf("failed to delete deployment %s: %w", name, err)
	}
	return nil
}

// StartPortForward starts a port forward to a pod or service
// Returns the PortForward struct which can be used to stop it later
func (m *Manager) StartPortForward(podName, localPort, remotePort string) (*PortForward, error) {
	// Use kubectl port-forward command for simplicity
	cmd := exec.Command("kubectl", "port-forward",
		"-n", m.namespace,
		fmt.Sprintf("pod/%s", podName),
		fmt.Sprintf("%s:%s", localPort, remotePort))

	// Start the command in the background
	err := cmd.Start()
	if err != nil {
		return nil, fmt.Errorf("failed to start port-forward: %w", err)
	}

	pf := &PortForward{
		PodName:    podName,
		LocalPort:  localPort,
		RemotePort: remotePort,
		Cmd:        cmd,
	}

	return pf, nil
}

// StopPortForward stops an active port forward
func (m *Manager) StopPortForward(pf *PortForward) error {
	if pf == nil || pf.Cmd == nil || pf.Cmd.Process == nil {
		return fmt.Errorf("invalid port forward")
	}

	err := pf.Cmd.Process.Kill()
	if err != nil {
		return fmt.Errorf("failed to stop port-forward: %w", err)
	}

	return nil
}
