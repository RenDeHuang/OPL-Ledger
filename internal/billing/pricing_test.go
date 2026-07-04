package billing

import "testing"

func TestPrepaidHoldCentsMatchesOplCloudBasicPackage(t *testing.T) {
	hold, err := PrepaidHoldCents(PricingInput{
		PackageID:         "basic",
		DiskGB:            10,
		ComputeHourlyCNY:  "0.39",
		StorageGBMonthCNY: "0.36",
		BillingMarkup:     "0.2",
		PrepaidHoldDays:   7,
	})
	if err != nil {
		t.Fatalf("prepaid hold: %v", err)
	}
	if hold.ComputeCents != 7862 {
		t.Fatalf("compute cents = %d", hold.ComputeCents)
	}
	if hold.StorageCents != 101 {
		t.Fatalf("storage cents = %d", hold.StorageCents)
	}
	if hold.TotalCents != 7963 {
		t.Fatalf("total cents = %d", hold.TotalCents)
	}
}

func TestPrepaidHoldCentsMatchesOplCloudProPackage(t *testing.T) {
	hold, err := PrepaidHoldCents(PricingInput{
		PackageID:         "pro",
		DiskGB:            100,
		ComputeHourlyCNY:  "3.09",
		StorageGBMonthCNY: "0.36",
		BillingMarkup:     "0.2",
		PrepaidHoldDays:   7,
	})
	if err != nil {
		t.Fatalf("prepaid hold: %v", err)
	}
	if hold.ComputeCents != 62294 {
		t.Fatalf("compute cents = %d", hold.ComputeCents)
	}
	if hold.StorageCents != 1008 {
		t.Fatalf("storage cents = %d", hold.StorageCents)
	}
	if hold.TotalCents != 63302 {
		t.Fatalf("total cents = %d", hold.TotalCents)
	}
}

func TestPrepaidHoldCentsRequiresPositiveHoldDays(t *testing.T) {
	_, err := PrepaidHoldCents(PricingInput{
		PackageID:         "basic",
		DiskGB:            10,
		ComputeHourlyCNY:  "0.39",
		StorageGBMonthCNY: "0.36",
		BillingMarkup:     "0.2",
		PrepaidHoldDays:   0,
	})
	if err == nil {
		t.Fatalf("expected error")
	}
}
