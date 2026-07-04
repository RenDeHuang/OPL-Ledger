package billing

import (
	"errors"
	"strconv"
	"strings"
)

type PricingInput struct {
	PackageID         string
	DiskGB            int64
	ComputeHourlyCNY  string
	StorageGBMonthCNY string
	BillingMarkup     string
	PrepaidHoldDays   int64
}

type PrepaidHold struct {
	PackageID    string `json:"packageId"`
	ComputeCents int64  `json:"computeCents"`
	StorageCents int64  `json:"storageCents"`
	TotalCents   int64  `json:"totalCents"`
	Days         int64  `json:"days"`
}

func PrepaidHoldCents(input PricingInput) (PrepaidHold, error) {
	if input.PrepaidHoldDays <= 0 {
		return PrepaidHold{}, errors.New("positive_hold_days_required")
	}
	if input.DiskGB < 0 {
		return PrepaidHold{}, errors.New("non_negative_disk_required")
	}
	computeHourly, err := parseCNY4(input.ComputeHourlyCNY)
	if err != nil {
		return PrepaidHold{}, err
	}
	storageGBMonth, err := parseCNY4(input.StorageGBMonthCNY)
	if err != nil {
		return PrepaidHold{}, err
	}
	markup, err := parseRatio4(input.BillingMarkup)
	if err != nil {
		return PrepaidHold{}, err
	}
	multiplier := int64(10000) + markup
	computeCNY4 := roundDiv(computeHourly*multiplier*24*input.PrepaidHoldDays, 10000)
	storageCNY4 := roundDiv(input.DiskGB*storageGBMonth*multiplier*input.PrepaidHoldDays, 10000*30)
	computeCents := cny4ToCents(computeCNY4)
	storageCents := cny4ToCents(storageCNY4)
	return PrepaidHold{
		PackageID:    input.PackageID,
		ComputeCents: computeCents,
		StorageCents: storageCents,
		TotalCents:   computeCents + storageCents,
		Days:         input.PrepaidHoldDays,
	}, nil
}

func parseCNY4(value string) (int64, error) {
	return parseDecimal4(value)
}

func parseRatio4(value string) (int64, error) {
	if strings.TrimSpace(value) == "" {
		return 2000, nil
	}
	return parseDecimal4(value)
}

func parseDecimal4(value string) (int64, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, nil
	}
	if strings.HasPrefix(trimmed, "-") {
		return 0, errors.New("negative_decimal_not_supported")
	}
	whole := trimmed
	fraction := ""
	if parts := strings.SplitN(trimmed, ".", 2); len(parts) == 2 {
		whole = parts[0]
		fraction = parts[1]
	}
	if whole == "" {
		whole = "0"
	}
	wholeValue, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, err
	}
	if len(fraction) > 4 {
		fraction = fraction[:4]
	}
	for len(fraction) < 4 {
		fraction += "0"
	}
	fractionValue := int64(0)
	if fraction != "" {
		fractionValue, err = strconv.ParseInt(fraction, 10, 64)
		if err != nil {
			return 0, err
		}
	}
	return wholeValue*10000 + fractionValue, nil
}

func cny4ToCents(value int64) int64 {
	return roundDiv(value, 100)
}

func roundDiv(numerator int64, denominator int64) int64 {
	if denominator <= 0 {
		return 0
	}
	return (numerator + denominator/2) / denominator
}
