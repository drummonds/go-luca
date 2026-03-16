package luca

import "testing"

func TestParseFullPath(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantType    AccountType
		wantProduct string
		wantID      string
		wantAddress string
		wantPending bool
		wantErr     bool
	}{
		{
			name:        "four-part path",
			input:       "Liability:InterestAccount:0000-111:Main",
			wantType:    AccountTypeLiability,
			wantProduct: "InterestAccount",
			wantID:      "0000-111",
			wantAddress: "Main",
		},
		{
			name:        "pending address",
			input:       "Liability:InterestAccount:0000-111:Pending",
			wantType:    AccountTypeLiability,
			wantProduct: "InterestAccount",
			wantID:      "0000-111",
			wantAddress: "Pending",
			wantPending: true,
		},
		{
			name:        "two-part path",
			input:       "Asset:Cash",
			wantType:    AccountTypeAsset,
			wantProduct: "Cash",
		},
		{
			name:        "three-part path",
			input:       "Equity:Capital:001",
			wantType:    AccountTypeEquity,
			wantProduct: "Capital",
			wantID:      "001",
		},
		{
			name:    "single component",
			input:   "Asset",
			wantErr: true,
		},
		{
			name:    "invalid type",
			input:   "Invalid:Something",
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotType, gotProduct, gotID, gotAddress, gotPending, err := parseFullPath(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotType != tt.wantType {
				t.Errorf("type = %q, want %q", gotType, tt.wantType)
			}
			if gotProduct != tt.wantProduct {
				t.Errorf("product = %q, want %q", gotProduct, tt.wantProduct)
			}
			if gotID != tt.wantID {
				t.Errorf("accountID = %q, want %q", gotID, tt.wantID)
			}
			if gotAddress != tt.wantAddress {
				t.Errorf("address = %q, want %q", gotAddress, tt.wantAddress)
			}
			if gotPending != tt.wantPending {
				t.Errorf("isPending = %v, want %v", gotPending, tt.wantPending)
			}
		})
	}
}

func TestBuildFullPath(t *testing.T) {
	tests := []struct {
		name    string
		aType   AccountType
		product string
		acctID  string
		address string
		want    string
	}{
		{"four parts", AccountTypeAsset, "Bank", "Current", "Main", "Asset:Bank:Current:Main"},
		{"two parts", AccountTypeAsset, "Cash", "", "", "Asset:Cash"},
		{"three parts", AccountTypeEquity, "Capital", "001", "", "Equity:Capital:001"},
		{"empty mid with address", AccountTypeLiability, "Savings", "", "Pending", "Liability:Savings::Pending"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildFullPath(tt.aType, tt.product, tt.acctID, tt.address)
			if got != tt.want {
				t.Errorf("BuildFullPath = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRebuildFullPath(t *testing.T) {
	a := &Account{
		Type:      AccountTypeAsset,
		Product:   "Bank",
		AccountID: "Current",
		Address:   "Main",
	}
	got := a.RebuildFullPath()
	if got != "Asset:Bank:Current:Main" {
		t.Errorf("RebuildFullPath = %q, want %q", got, "Asset:Bank:Current:Main")
	}
	if a.FullPath != got {
		t.Errorf("FullPath not set, got %q", a.FullPath)
	}
}
