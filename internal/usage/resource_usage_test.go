package usage

import "testing"

func TestResourceUsageLogForComputeCarriesComputeAndWorkspaceIDs(t *testing.T) {
	log := NewResourceUsageLog(ResourceUsageInput{
		AccountID:      "acct_1",
		UserID:         "usr_1",
		WorkspaceID:    "ws_1",
		ComputeID:      "compute_1",
		ResourceKind:   ResourceKindCompute,
		Quantity:       1,
		Unit:           "hour",
		UnitPriceCents: 47,
		AmountCents:    47,
		SourceEventID:  "billing_tick_1",
	})

	if log.ID == "" {
		t.Fatalf("expected generated id")
	}
	if log.WorkspaceID != "ws_1" || log.ComputeID != "compute_1" {
		t.Fatalf("compute log ids = %+v", log)
	}
	if log.ResourceKind != ResourceKindCompute || log.AmountCents != 47 {
		t.Fatalf("compute log = %+v", log)
	}
}

func TestResourceUsageLogForStorageCarriesStorageAttachmentAndWorkspaceIDs(t *testing.T) {
	log := NewResourceUsageLog(ResourceUsageInput{
		AccountID:      "acct_1",
		UserID:         "usr_1",
		WorkspaceID:    "ws_1",
		StorageID:      "storage_1",
		AttachmentID:   "attach_1",
		ResourceKind:   ResourceKindStorage,
		Quantity:       10,
		Unit:           "gb_hour",
		UnitPriceCents: 1,
		AmountCents:    1,
		SourceEventID:  "billing_tick_1",
	})

	if log.WorkspaceID != "ws_1" || log.StorageID != "storage_1" || log.AttachmentID != "attach_1" {
		t.Fatalf("storage log ids = %+v", log)
	}
	if log.ResourceKind != ResourceKindStorage {
		t.Fatalf("storage log = %+v", log)
	}
}
