package k8s

import (
	"context"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type Snapshot struct {
	ClusterID          string         `json:"clusterId"`
	Namespace          string         `json:"namespace"`
	ObjectKind         string         `json:"objectKind"`
	ObjectName         string         `json:"objectName"`
	WorkspaceID        string         `json:"workspaceId,omitempty"`
	ResourceVersion    string         `json:"resourceVersion"`
	ObservedGeneration int64          `json:"observedGeneration"`
	ReadinessStatus    string         `json:"readinessStatus"`
	CollectedAt        time.Time      `json:"collectedAt"`
	RedactedObject     map[string]any `json:"redactedObject"`
}

type Collector struct {
	client kubernetes.Interface
}

type SnapshotStore interface {
	AppendKubernetesEvidenceSnapshot(ctx context.Context, snapshot Snapshot) (Snapshot, error)
}

func NewCollector(client kubernetes.Interface) *Collector {
	return &Collector{client: client}
}

func (c *Collector) CollectAndPersistDeployment(ctx context.Context, store SnapshotStore, clusterID string, namespace string, name string) (Snapshot, error) {
	snapshot, err := c.CollectDeployment(ctx, clusterID, namespace, name)
	if err != nil {
		return Snapshot{}, err
	}
	return store.AppendKubernetesEvidenceSnapshot(ctx, snapshot)
}

func (c *Collector) CollectDeployment(ctx context.Context, clusterID string, namespace string, name string) (Snapshot, error) {
	deployment, err := c.client.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return Snapshot{}, err
	}
	status := "not_ready"
	if deployment.Status.ReadyReplicas >= 1 && deployment.Status.ReadyReplicas == deployment.Status.Replicas {
		status = "ready"
	}
	labels := map[string]string{}
	for key, value := range deployment.Labels {
		labels[key] = value
	}
	return Snapshot{
		ClusterID:          clusterID,
		Namespace:          namespace,
		ObjectKind:         "Deployment",
		ObjectName:         name,
		WorkspaceID:        labels["oplcloud.cn/workspace-id"],
		ResourceVersion:    deployment.ResourceVersion,
		ObservedGeneration: deployment.Status.ObservedGeneration,
		ReadinessStatus:    status,
		CollectedAt:        time.Now().UTC(),
		RedactedObject: map[string]any{
			"apiVersion":    "apps/v1",
			"kind":          "Deployment",
			"name":          deployment.Name,
			"namespace":     deployment.Namespace,
			"labels":        labels,
			"readyReplicas": deployment.Status.ReadyReplicas,
			"replicas":      deployment.Status.Replicas,
		},
	}, nil
}
