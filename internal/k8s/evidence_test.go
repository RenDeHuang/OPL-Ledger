package k8s

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestCollectDeploymentEvidenceReadsStatusWithoutSecrets(t *testing.T) {
	client := fake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "opl-ws-1",
				Namespace:       "opl-cloud",
				ResourceVersion: "42",
				Labels:          map[string]string{"oplcloud.cn/workspace-id": "ws_1"},
			},
			Status: appsv1.DeploymentStatus{ReadyReplicas: 1, Replicas: 1, ObservedGeneration: 7},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "opl-ws-1-token", Namespace: "opl-cloud"},
			Data:       map[string][]byte{"token": []byte("secret-value")},
		},
	)
	collector := NewCollector(client)
	snapshot, err := collector.CollectDeployment(context.Background(), "cluster_1", "opl-cloud", "opl-ws-1")
	if err != nil {
		t.Fatalf("collect deployment: %v", err)
	}
	if snapshot.ReadinessStatus != "ready" {
		t.Fatalf("expected ready, got %s", snapshot.ReadinessStatus)
	}
	if snapshot.WorkspaceID != "ws_1" {
		t.Fatalf("expected ws_1, got %s", snapshot.WorkspaceID)
	}
	if snapshot.RedactedObject["secretData"] != nil {
		t.Fatalf("snapshot leaked secret data")
	}
}
