// Package k8s provides a k8s client.
package k8s

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"google.golang.org/grpc"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
	podresourcesapi "k8s.io/kubelet/pkg/apis/podresources/v1"
)

// We inject this env var into pod.
const (
	nodenameEnvVar = "NODE_NAME"
)

// Maximum gRPC receive message size.
const (
	grpcClientRecvSizeMax = 128 * 1024 * 1024
)

type Client struct {
	Logger            *slog.Logger
	Hostname          string
	Config            *rest.Config
	Clientset         kubernetes.Interface
	PodResourceClient podresourcesapi.PodResourcesListerClient
	cleanup           func() error
}

// Device is a representation of a allocatable device for k8s containers.
type Device struct {
	Name string
	IDs  []string
}

// Container is a representation of K8s container resource.
type Container struct {
	Name    string
	UID     string
	Devices []Device
}

// Pod is a representation of K8s pod resource.
type Pod struct {
	Namespace  string
	Name       string
	UID        string
	CreatedAt  time.Time
	StartedAt  time.Time
	DeletedAt  time.Time
	Status     string
	QoS        string
	Containers []Container
}

// New returns an instance of Client struct.
func New(kubeconfigPath string, kubeletSocket string, logger *slog.Logger) (*Client, error) {
	var config *rest.Config

	var err error

	// If configFile is not found, it will fallback to in cluster config
	if kubeconfigPath == "" {
		logger.Debug("Falling back to in-cluster k8s config")

		config, err = rest.InClusterConfig()
	} else {
		logger.Debug("Creating k8s config using provided config file", "path", kubeconfigPath)

		config, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath}, nil,
		).ClientConfig()
	}

	if err != nil {
		return nil, err
	}

	// creates the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	// If kubelet socket is mounted, create a pod resource client
	var client podresourcesapi.PodResourcesListerClient

	var cleanup func() error

	if _, err := os.Stat(kubeletSocket); err == nil {
		conn, err := ConnectToServer(kubeletSocket)
		if err != nil {
			return nil, err
		}

		client = podresourcesapi.NewPodResourcesListerClient(conn)

		// Close connection when stopping client
		cleanup = func() error {
			return conn.Close()
		}
	}

	return &Client{
		Logger:            logger,
		Hostname:          os.Getenv(nodenameEnvVar),
		Config:            config,
		Clientset:         clientset,
		PodResourceClient: client,
		cleanup:           cleanup,
	}, nil
}

// Close stops clients and release resources.
func (c *Client) Close() error {
	if c.cleanup != nil {
		c.Logger.Debug("Closing pod resources lister client")

		return c.cleanup()
	}

	return nil
}

// Pods returns current pods in the cluster.
func (c *Client) Pods(ctx context.Context, ns string, opts metav1.ListOptions) ([]Pod, error) {
	resp, err := c.Clientset.CoreV1().Pods(ns).List(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to get pods: %w", err)
	}

	// Make a slice of pods
	pods := make([]Pod, len(resp.Items))
	for i, pod := range resp.Items {
		pods[i] = Pod{
			Namespace: pod.GetNamespace(),
			Name:      pod.GetName(),
			UID:       string(pod.GetUID()),
		}

		// Add containers to pod
		for _, cont := range pod.Spec.Containers {
			pods[i].Containers = append(pods[i].Containers, Container{Name: cont.Name})
		}
	}

	return pods, nil
}

// PodDevices returns a slice of pods with devices associated with each pod.
func (c *Client) PodDevices(ctx context.Context) ([]Pod, error) {
	if c.PodResourceClient == nil {
		return nil, errors.New("pod resource API is not available")
	}

	// If hostname is not empty, get only pods running on current host
	opts := metav1.ListOptions{}
	if c.Hostname != "" {
		opts = metav1.ListOptions{
			FieldSelector: "spec.nodeName=" + c.Hostname,
		}
	}

	// Get pods from all namespaces on the current node
	pods, err := c.Pods(ctx, "", opts)
	if err != nil {
		return nil, err
	}

	// Get pod resources
	// We set maximum message receive size to 128 MiB just in case if there are too many
	// pods running on the node. Should be ok for most of the production cases. Default when
	// is 4 MiB.
	resp, err := c.PodResourceClient.List(ctx, &podresourcesapi.ListPodResourcesRequest{}, grpc.MaxCallRecvMsgSize(grpcClientRecvSizeMax))
	if err != nil {
		return nil, fmt.Errorf("failed to get pod resources: %w", err)
	}

	return mergePodResources(pods, resp), nil
}

// Exec executes a given command in the pod and returns output.
func (c *Client) Exec(ctx context.Context, ns string, pod string, container string, cmd []string) ([]byte, []byte, error) {
	req := c.Clientset.CoreV1().RESTClient().Post().Resource("pods").Name(pod).Namespace(ns).SubResource("exec")

	// Set pod exec options
	opts := &v1.PodExecOptions{
		Command:   cmd,
		Container: container,
		Stdout:    true,
		Stderr:    true,
	}

	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		return nil, nil, fmt.Errorf("failed to add to scheme: %w", err)
	}

	req.VersionedParams(
		opts, runtime.NewParameterCodec(scheme),
	)

	// Execute command in pod
	exec, err := remotecommand.NewSPDYExecutor(c.Config, "POST", req.URL())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create executor: %w", err)
	}

	var stdout, stderr bytes.Buffer

	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
		Tty:    false,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("error in stream: %w", err)
	}

	return stdout.Bytes(), stderr.Bytes(), nil
}

// mergePodResources merges the existing Pod resources, if and when found, to Pod struct.
func mergePodResources(pods []Pod, resp *podresourcesapi.ListPodResourcesResponse) []Pod {
	// Make a map of pods for easy lookup
	podsMap := make(map[string]int, len(pods))
	for i, pod := range pods {
		podsMap[pod.Name] = i
	}

	// Loop over resources and add container and devices info to Pods.
	for _, p := range resp.GetPodResources() {
		var podIndx int

		var ok bool
		if podIndx, ok = podsMap[p.GetName()]; !ok {
			continue
		}

		for _, c := range p.GetContainers() {
			container := Container{
				Name: c.GetName(),
			}

			for _, d := range c.GetDevices() {
				container.Devices = append(container.Devices, Device{
					Name: d.GetResourceName(),
					IDs:  d.GetDeviceIds(),
				})
			}

			pods[podIndx].Containers = append(pods[podIndx].Containers, container)
		}
	}

	return pods
}
