package pod

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestListFromKubeletReadOnlyPort(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/pods" {
			t.Errorf("expected to request '/pods', got: %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("expected GET request, got: %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		http.ServeFile(w, r, "kubelet-readonly-pods.json")
	}))
	defer srv.Close()

	portRaw := srv.URL[len("http://127.0.0.1:"):]
	port, _ := strconv.ParseInt(portRaw, 10, 32)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pods, err := ListFromKubeletReadOnlyPort(ctx, int(port))
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if pods == nil {
		t.Fatal("expected non-nil result")
	}
	if len(pods.Items) != 2 {
		t.Fatalf("expected 2 pod, got %d", len(pods.Items))
	}
	if pods.Items[0].Name != "vector-jldbs" {
		t.Fatalf("expected pod name to be vector-jldbs, got %s", pods.Items[0].Name)
	}
	if pods.Items[0].Status.Phase != corev1.PodRunning {
		t.Errorf("expected pod phase 'Running', got: %s", pods.Items[0].Status.Phase)
	}
	if pods.Items[1].Name != "kube-proxy-hfqwt" {
		t.Fatalf("expected pod name to be kube-proxy-hfqwt, got %s", pods.Items[1].Name)
	}
	if pods.Items[1].Status.Phase != corev1.PodRunning {
		t.Errorf("expected pod phase 'Running', got: %s", pods.Items[1].Status.Phase)
	}
}

func TestGetFromKubeletReadOnlyPort_Error(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	portRaw := srv.URL[len("http://127.0.0.1:"):]
	port, _ := strconv.ParseInt(portRaw, 10, 32)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result, err := ListFromKubeletReadOnlyPort(ctx, int(port))

	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if result != nil {
		t.Fatalf("expected nil result, got: %v", result)
	}
}

func Test_parsePodsFromKubeletReadOnlyPort(t *testing.T) {
	t.Parallel()

	file, err := os.OpenFile("kubelet-readonly-pods.json", os.O_RDONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	pods, err := parsePodsFromKubeletReadOnlyPort(file)
	if err != nil {
		t.Fatal(err)
	}
	if pods == nil {
		t.Fatal("expected non-nil result")
	}
	if len(pods.Items) != 2 {
		t.Fatalf("expected 2 pod, got %d", len(pods.Items))
	}
	if pods.Items[0].Name != "vector-jldbs" {
		t.Fatalf("expected pod name to be vector-jldbs, got %s", pods.Items[0].Name)
	}
	if pods.Items[0].Status.Phase != corev1.PodRunning {
		t.Errorf("expected pod phase 'Running', got: %s", pods.Items[0].Status.Phase)
	}
	if pods.Items[1].Name != "kube-proxy-hfqwt" {
		t.Fatalf("expected pod name to be kube-proxy-hfqwt, got %s", pods.Items[1].Name)
	}
	if pods.Items[1].Status.Phase != corev1.PodRunning {
		t.Errorf("expected pod phase 'Running', got: %s", pods.Items[1].Status.Phase)
	}
}
