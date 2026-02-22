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
