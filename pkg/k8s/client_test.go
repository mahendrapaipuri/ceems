package k8s

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	podresourcesapi "k8s.io/kubelet/pkg/apis/podresources/v1"
)

var noOpLogger = slog.New(slog.DiscardHandler)

var (
	testPods = []runtime.Object{
		&v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod11",
				UID:       "uid11",
				Namespace: "ns1",
				Labels: map[string]string{
					"label1": "value1",
				},
			},
		},
		&v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod12",
				UID:       "uid12",
				Namespace: "ns1",
				Labels: map[string]string{
					"label1": "value1",
				},
			},
		},
		&v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod21",
				UID:       "uid21",
				Namespace: "ns2",
				Labels: map[string]string{
					"label2": "value2",
				},
			},
		},
	}

	testRoleBindings = []runtime.Object{
		&rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rb1",
				UID:       "rb1",
				Namespace: "foo",
			},
			Subjects: []rbacv1.Subject{
				{
					Kind:      "User",
					Name:      "foo",
					Namespace: "foo",
				},
				{
					Kind:      "User",
					Name:      "fooplus",
					Namespace: "foo",
				},
				{
					Kind:      "Group",
					Name:      "foogroup",
					Namespace: "foo",
				},
				{
					Kind:      "ServiceAccount",
					Name:      "foosvc",
					Namespace: "foo",
				},
			},
		},
		&rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rb2",
				UID:       "rb2",
				Namespace: "foo",
			},
			Subjects: []rbacv1.Subject{
				{
					Kind:      "User",
					Name:      "bar",
					Namespace: "foo",
				},
			},
		},
		&rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rb3",
				UID:       "rb3",
				Namespace: "bar",
			},
			Subjects: []rbacv1.Subject{
				{
					Kind:      "User",
					Name:      "barplus",
					Namespace: "bar",
				},
				{
					Kind:      "User",
					Name:      "fooplus",
					Namespace: "bar",
				},
			},
		},
	}
)

func TestNew(t *testing.T) {
	content := `
apiVersion: v1
clusters:
- cluster:
    server: https://localhost:8080
  name: foo-cluster
contexts:
- context:
    cluster: foo-cluster
    user: foo-user
    namespace: bar
  name: foo-context
current-context: foo-context
kind: Config
users:
- name: foo-user
  user:
    token: blue-token
`
	tmpfile, err := os.CreateTemp(t.TempDir(), "kubeconfig")
	require.NoError(t, err)

	err = os.WriteFile(tmpfile.Name(), []byte(content), 0o700) //nolint:gosec
	require.NoError(t, err)

	// Create fake kubelet socket server
	socketDir := t.TempDir()
	kubelet, err := FakeKubeletServer(socketDir, nil, nil)
	require.NoError(t, err)

	defer kubelet.Server.Stop()

	c, err := New(tmpfile.Name(), filepath.Join(socketDir, "kubelet.sock"), noOpLogger)
	require.NoError(t, err)

	err = c.Close()
	require.NoError(t, err)
}

func TestPodsLister(t *testing.T) {
	testCases := []struct {
		name     string
		targetNS string
		opts     metav1.ListOptions
		expected []Pod
	}{
		{
			name: "List all pods",
			expected: []Pod{
				{
					Namespace: "ns1",
					Name:      "pod11",
					UID:       "uid11",
				},
				{
					Namespace: "ns1",
					Name:      "pod12",
					UID:       "uid12",
				},
				{
					Namespace: "ns2",
					Name:      "pod21",
					UID:       "uid21",
				},
			},
		},
		{
			name:     "List pods in namespace",
			targetNS: "ns2",
			expected: []Pod{
				{
					Namespace: "ns2",
					Name:      "pod21",
					UID:       "uid21",
				},
			},
		},
		{
			name: "Get pod(s) based on label",
			opts: metav1.ListOptions{
				LabelSelector: "label1=value1",
			},
			expected: []Pod{
				{
					Namespace: "ns1",
					Name:      "pod11",
					UID:       "uid11",
				},
				{
					Namespace: "ns1",
					Name:      "pod12",
					UID:       "uid12",
				},
			},
		},
	}

	// Make fake client
	fakeClientset := fake.NewClientset(testPods...)

	// Make k8s client
	client := &Client{
		Logger:    noOpLogger,
		Clientset: fakeClientset,
	}

	for _, test := range testCases {
		got, err := client.ListPods(t.Context(), test.targetNS, test.opts)
		require.NoError(t, err)

		assert.ElementsMatch(t, test.expected, got, test.name)
	}
}

func TestInformer(t *testing.T) {
	// Make fake client
	fakeClientset := fake.NewClientset(testPods...)

	// Make k8s client
	client := &Client{
		Logger:    noOpLogger,
		Clientset: fakeClientset,
		pods:      make(map[string]*v1.Pod),
		stopCh:    make(chan struct{}),
	}

	// Make new informer
	err := client.NewPodInformer(time.Second)
	require.NoError(t, err)

	// Start informer
	err = client.StartInformer()
	require.NoError(t, err)

	// Sleep a while to fetch all pods
	time.Sleep(time.Second)

	// Fetch current pods
	pods := client.Pods()

	// Check fetched pods
	assert.Len(t, pods, len(testPods))

	// Stop informer
	client.Close()
}

func TestPodDevices(t *testing.T) {
	podResourcesResp := &podresourcesapi.ListPodResourcesResponse{
		PodResources: []*podresourcesapi.PodResources{
			{
				Name:      "pod11",
				Namespace: "ns1",
				Containers: []*podresourcesapi.ContainerResources{
					{
						Name: "cont110",
						Devices: []*podresourcesapi.ContainerDevices{
							{
								ResourceName: "nvidia.com/gpu",
								DeviceIds:    []string{"GPU-asdasdas-f453-56a4-b023-2a7baa2557a7", "GPU-asdasdas-f453-sdfsdf-b023-2a7baa2557a7"},
							},
						},
					},
					{
						Name: "cont111",
						Devices: []*podresourcesapi.ContainerDevices{
							{
								ResourceName: "nvidia.com/gpu",
								DeviceIds:    []string{"MIG-ef49409b-f453-56a4-b023-2a7baa2557a7::0"},
							},
						},
					},
				},
			},
			{
				Name:      "pod21",
				Namespace: "ns2",
				Containers: []*podresourcesapi.ContainerResources{
					{
						Name: "cont210",
						Devices: []*podresourcesapi.ContainerDevices{
							{
								ResourceName: "amd.com/gpu",
								DeviceIds:    []string{"0000:c8:00.0"},
							},
						},
					},
				},
			},
		},
	}

	expected := []Pod{
		{
			Namespace: "ns1",
			Name:      "pod11",
			UID:       "uid11",
			Containers: []Container{
				{
					Name: "cont110",
					Devices: []Device{
						{
							Name: "nvidia.com/gpu",
							IDs:  []string{"GPU-asdasdas-f453-56a4-b023-2a7baa2557a7", "GPU-asdasdas-f453-sdfsdf-b023-2a7baa2557a7"},
						},
					},
				},
				{
					Name: "cont111",
					Devices: []Device{
						{
							Name: "nvidia.com/gpu",
							IDs:  []string{"MIG-ef49409b-f453-56a4-b023-2a7baa2557a7::0"},
						},
					},
				},
			},
		},
		{
			Namespace: "ns1",
			Name:      "pod12",
			UID:       "uid12",
		},
		{
			Namespace: "ns2",
			Name:      "pod21",
			UID:       "uid21",
			Containers: []Container{
				{
					Name: "cont210",
					Devices: []Device{
						{
							Name: "amd.com/gpu",
							IDs:  []string{"0000:c8:00.0"},
						},
					},
				},
			},
		},
	}

	// Create fake kubelet socket server
	socketDir := t.TempDir()
	kubelet, err := FakeKubeletServer(socketDir, podResourcesResp, nil)
	require.NoError(t, err)

	defer kubelet.Server.Stop()

	// Create connection
	conn, err := ConnectToServer(filepath.Join(socketDir, "kubelet.sock"))
	require.NoError(t, err)

	defer conn.Close()

	kubeletClient := podresourcesapi.NewPodResourcesListerClient(conn)

	// Make fake client
	fakeClientset := fake.NewSimpleClientset(testPods...)

	// Make k8s client
	client := &Client{
		Logger:            noOpLogger,
		Clientset:         fakeClientset,
		PodResourceClient: kubeletClient,
	}

	got, err := client.ListPodsWithDevs(t.Context())
	require.NoError(t, err)

	assert.ElementsMatch(t, expected, got)
}

func TestPodExec(t *testing.T) {
	expected := `GPU 0: NVIDIA H100 80GB HBM3 (UUID: GPU-0508b50f-5b05-50cc-9e27-a5a83f666c25)
GPU 1: NVIDIA H100 80GB HBM3 (UUID: GPU-f313a2fd-e11f-d5ed-53c2-2d3b3a1a3a6c)
GPU 2: NVIDIA H100 80GB HBM3 (UUID: GPU-82eb82a7-f57a-3b59-65b1-678434875eb4)
GPU 3: NVIDIA H100 80GB HBM3 (UUID: GPU-2114ac3c-d010-ef91-2ab8-45544c7b64c5)`

	// Create fake SPDY server.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var stdout, stderr bytes.Buffer

		ctx, err := CreateHTTPStreams(w, req, &remotecommand.StreamOptions{
			Stdout: &stdout,
			Stderr: &stderr,
		})
		if err != nil {
			w.WriteHeader(http.StatusForbidden)

			return
		}

		defer ctx.conn.Close()

		r := io.NopCloser(strings.NewReader(expected))

		_, err = io.Copy(ctx.stdoutStream, r)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)

			return
		}
	}))
	defer server.Close()

	// Make k8s client
	client := &Client{
		Logger:    noOpLogger,
		Clientset: kubernetes.NewForConfigOrDie(&rest.Config{Host: server.URL}),
		Config:    &rest.Config{Host: server.URL},
	}

	stdout, stderr, err := client.Exec(t.Context(), "ns1", "pod11", "cont110", []string{"nvidia-smi", "-L"})
	require.NoError(t, err)
	assert.Equal(t, expected, string(stdout))
	assert.Empty(t, stderr)
}

func TestListUsers(t *testing.T) {
	// Make fake client
	fakeClientset := fake.NewClientset(testRoleBindings...)

	// Make k8s client
	client := &Client{
		Logger:    noOpLogger,
		Clientset: fakeClientset,
		pods:      make(map[string]*v1.Pod),
		stopCh:    make(chan struct{}),
	}

	// Fetch users
	usersMap, err := client.ListUsers(t.Context(), "")
	require.NoError(t, err)

	// Expected
	expectedUsersMap := map[string][]string{
		"foo": {"foo", "fooplus", "bar"},
		"bar": {"barplus", "fooplus"},
	}

	// Check fetched pods
	assert.Equal(t, expectedUsersMap, usersMap)
}

func TestConfigMap(t *testing.T) {
	// Expected data
	expectedData := map[string]string{
		"test": "value",
	}

	// Make fake client
	fakeClientset := fake.NewClientset(
		[]runtime.Object{
			&v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cm",
					UID:       "cm",
					Namespace: "default",
				},
				Data: expectedData,
			},
		}...,
	)

	// Make k8s client
	client := &Client{
		Logger:    noOpLogger,
		Clientset: fakeClientset,
		pods:      make(map[string]*v1.Pod),
		stopCh:    make(chan struct{}),
	}

	// Fetch config
	cm, err := client.ConfigMap(t.Context(), "default", "cm")
	require.NoError(t, err)

	// Check config
	assert.Equal(t, expectedData, cm)

	// Try to fetch non existent cm
	_, err = client.ConfigMap(t.Context(), "foo", "bar")
	require.Error(t, err)
}
