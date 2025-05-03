package k8s

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
	"google.golang.org/grpc"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/httpstream"
	"k8s.io/apimachinery/pkg/util/httpstream/spdy"
	remotecommandconsts "k8s.io/apimachinery/pkg/util/remotecommand"
	"k8s.io/client-go/tools/remotecommand"
	podresourcesapi "k8s.io/kubelet/pkg/apis/podresources/v1"
)

type streamContext struct {
	conn         io.Closer
	stdinStream  io.ReadCloser
	stdoutStream io.WriteCloser
	stderrStream io.WriteCloser
	writeStatus  func(status *apierrors.StatusError) error
}

type streamAndReply struct {
	httpstream.Stream
	replySent <-chan struct{}
}

func v4WriteStatusFunc(stream io.Writer) func(status *apierrors.StatusError) error {
	return func(status *apierrors.StatusError) error {
		bs, err := json.Marshal(status.Status())
		if err != nil {
			return err
		}

		_, err = stream.Write(bs)

		return err
	}
}

// CreateHTTPStreams only support StreamProtocolV4Name
// Nicked from https://github.com/kubernetes/client-go/blob/c106b23895edc59ff05c65770a17e6a6d3caee66/tools/remotecommand/spdy_test.go#L227-L288
func CreateHTTPStreams(w http.ResponseWriter, req *http.Request, opts *remotecommand.StreamOptions) (*streamContext, error) {
	_, err := httpstream.Handshake(req, w, []string{remotecommandconsts.StreamProtocolV4Name})
	if err != nil {
		return nil, err
	}

	upgrader := spdy.NewResponseUpgrader()
	streamCh := make(chan streamAndReply)
	conn := upgrader.UpgradeResponse(w, req, func(stream httpstream.Stream, replySent <-chan struct{}) error {
		streamCh <- streamAndReply{Stream: stream, replySent: replySent}

		return nil
	})
	ctx := &streamContext{
		conn: conn,
	}

	// wait for stream
	replyChan := make(chan struct{}, 4)
	defer close(replyChan)

	receivedStreams := 0

	expectedStreams := 1
	if opts.Stdout != nil {
		expectedStreams++
	}

	if opts.Stdin != nil {
		expectedStreams++
	}

	if opts.Stderr != nil {
		expectedStreams++
	}
WaitForStreams:
	for {
		select {
		case stream := <-streamCh:
			streamType := stream.Headers().Get(v1.StreamType)
			switch streamType {
			case v1.StreamTypeError:
				replyChan <- struct{}{}
				ctx.writeStatus = v4WriteStatusFunc(stream)
			case v1.StreamTypeStdout:
				replyChan <- struct{}{}
				ctx.stdoutStream = stream
			case v1.StreamTypeStdin:
				replyChan <- struct{}{}
				ctx.stdinStream = stream
			case v1.StreamTypeStderr:
				replyChan <- struct{}{}
				ctx.stderrStream = stream
			default:
				// add other stream ...
				return nil, errors.New("unimplemented stream type")
			}
		case <-replyChan:
			receivedStreams++
			if receivedStreams == expectedStreams {
				break WaitForStreams
			}
		}
	}

	return ctx, nil
}

type FakeResourceServer struct {
	Server                      *grpc.Server
	ListResp                    *podresourcesapi.ListPodResourcesResponse
	GetAllocatableResourcesResp *podresourcesapi.AllocatableResourcesResponse
}

func (m *FakeResourceServer) GetAllocatableResources(_ context.Context, _ *podresourcesapi.AllocatableResourcesRequest) (*podresourcesapi.AllocatableResourcesResponse, error) {
	return m.GetAllocatableResourcesResp, nil
}

func (m *FakeResourceServer) List(_ context.Context, _ *podresourcesapi.ListPodResourcesRequest) (*podresourcesapi.ListPodResourcesResponse, error) {
	return m.ListResp, nil
}

func (m *FakeResourceServer) Get(_ context.Context, _ *podresourcesapi.GetPodResourcesRequest) (*podresourcesapi.GetPodResourcesResponse, error) {
	return &podresourcesapi.GetPodResourcesResponse{}, nil
}

// CreateListener creates a listener on the specified endpoint.
// based from k8s.io/kubernetes/pkg/kubelet/util
// Nicked from https://github.com/k8snetworkplumbingwg/multus-cni/blob/v4.2.0/pkg/kubeletclient/kubeletclient_test.go
func CreateListener(addr string) (net.Listener, error) {
	// Unlink to cleanup the previous socket file.
	err := unix.Unlink(addr)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to unlink socket file %q: %w", addr, err)
	}

	if err := os.MkdirAll(filepath.Dir(addr), 0o750); err != nil {
		return nil, fmt.Errorf("error creating socket directory %q: %w", filepath.Dir(addr), err)
	}

	// Create the socket on a tempfile and move it to the destination socket to handle improper cleanup
	file, err := os.CreateTemp(filepath.Dir(addr), "")
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary file: %w", err)
	}

	if err := os.Remove(file.Name()); err != nil {
		return nil, fmt.Errorf("failed to remove temporary file: %w", err)
	}

	l, err := net.Listen("unix", file.Name())
	if err != nil {
		return nil, err
	}

	if err = os.Rename(file.Name(), addr); err != nil {
		return nil, fmt.Errorf("failed to move temporary file to addr %q: %w", addr, err)
	}

	return l, nil
}

// FakeKubeletServer returns a mock API resource server.
func FakeKubeletServer(socketDir string, listResp *podresourcesapi.ListPodResourcesResponse, getAllocatableResourcesResp *podresourcesapi.AllocatableResourcesResponse) (*FakeResourceServer, error) {
	// Ensure socket directory exists
	if err := os.MkdirAll(socketDir, os.ModeDir); err != nil {
		return nil, err
	}

	socketName := filepath.Join(socketDir, "kubelet.sock")
	fakeServer := &FakeResourceServer{Server: grpc.NewServer(), ListResp: listResp, GetAllocatableResourcesResp: getAllocatableResourcesResp}
	podresourcesapi.RegisterPodResourcesListerServer(fakeServer.Server, fakeServer)

	lis, err := CreateListener(socketName)
	if err != nil {
		return nil, err
	}

	go fakeServer.Server.Serve(lis) //nolint:errcheck

	return fakeServer, nil
}
