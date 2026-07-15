package net

import "testing"

func TestVerifyMeshwireInterface(t *testing.T) {
	tests := []struct {
		name    string
		addr    string
		wantOK  bool
		wantErr bool
	}{
		{name: "in-range internal address", addr: "10.64.1.5", wantOK: true, wantErr: false},
		{name: "public address out of scope", addr: "8.8.8.8", wantOK: false, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, err := VerifyMeshwireInterface(tt.addr)
			if ok != tt.wantOK {
				t.Errorf("VerifyMeshwireInterface(%q) ok = %v, want %v", tt.addr, ok, tt.wantOK)
			}
			if (err != nil) != tt.wantErr {
				t.Errorf("VerifyMeshwireInterface(%q) err = %v, wantErr %v", tt.addr, err, tt.wantErr)
			}
		})
	}
}
