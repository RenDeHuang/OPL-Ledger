package k8s

import (
	"context"
	"encoding/json"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

type recordingSnapshotStore struct {
	snapshots []Snapshot
}

func (s *recordingSnapshotStore) AppendKubernetesEvidenceSnapshot(_ context.Context, snapshot Snapshot) (Snapshot, error) {
	s.snapshots = append(s.snapshots, snapshot)
	return snapshot, nil
}

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

func TestCollectAndPersistDeploymentSnapshotStoresRedactedObject(t *testing.T) {
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
	store := &recordingSnapshotStore{}

	snapshot, err := collector.CollectAndPersistDeployment(context.Background(), store, "cluster_1", "opl-cloud", "opl-ws-1")
	if err != nil {
		t.Fatalf("collect and persist deployment: %v", err)
	}
	if len(store.snapshots) != 1 {
		t.Fatalf("expected 1 persisted snapshot, got %d", len(store.snapshots))
	}
	if store.snapshots[0].RedactedObject["name"] != "opl-ws-1" || store.snapshots[0].RedactedObject["readyReplicas"] != int32(1) {
		t.Fatalf("redacted object = %+v", store.snapshots[0].RedactedObject)
	}
	if snapshot.ObjectName != store.snapshots[0].ObjectName {
		t.Fatalf("returned snapshot was not persisted: returned=%+v persisted=%+v", snapshot, store.snapshots[0])
	}
}

func TestCollectAndPersistDeploymentSnapshotNeverPersistsSecretValues(t *testing.T) {
	client := fake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "opl-ws-1",
				Namespace: "opl-cloud",
				Labels:    map[string]string{"oplcloud.cn/workspace-id": "ws_1"},
			},
			Status: appsv1.DeploymentStatus{ReadyReplicas: 1, Replicas: 1},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "opl-ws-1-token", Namespace: "opl-cloud"},
			Data:       map[string][]byte{"token": []byte("secret-value")},
		},
	)
	collector := NewCollector(client)
	store := &recordingSnapshotStore{}

	_, err := collector.CollectAndPersistDeployment(context.Background(), store, "cluster_1", "opl-cloud", "opl-ws-1")
	if err != nil {
		t.Fatalf("collect and persist deployment: %v", err)
	}
	payload, err := json.Marshal(store.snapshots[0].RedactedObject)
	if err != nil {
		t.Fatalf("marshal redacted object: %v", err)
	}
	if string(payload) == "" || json.Valid(payload) == false {
		t.Fatalf("invalid persisted payload: %s", payload)
	}
	if containsSecretValue(string(payload), "secret-value") {
		t.Fatalf("persisted payload leaked secret value: %s", payload)
	}
}

func containsSecretValue(payload string, secret string) bool {
	return len(secret) > 0 && len(payload) >= len(secret) && jsonContains(payload, secret)
}

func jsonContains(payload string, needle string) bool {
	for i := 0; i+len(needle) <= len(payload); i++ {
		if payload[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
