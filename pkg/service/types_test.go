package service

import (
	"encoding/json"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestUnmarshalDiskInfo(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		want    DiskInfo
		wantErr bool
	}{
		{
			name: "valid disk info with single disk",
			data: `{"disks":[{"imageName":"ubuntu-20.04","size":20}]}`,
			want: DiskInfo{
				Disks: []Disk{
					{ImageName: "ubuntu-20.04", Size: 20},
				},
			},
			wantErr: false,
		},
		{
			name: "valid disk info with multiple disks",
			data: `{"disks":[{"imageName":"ubuntu-20.04","size":20},{"imageName":"data-disk","size":100}]}`,
			want: DiskInfo{
				Disks: []Disk{
					{ImageName: "ubuntu-20.04", Size: 20},
					{ImageName: "data-disk", Size: 100},
				},
			},
			wantErr: false,
		},
		{
			name:    "empty disk info",
			data:    `{"disks":[]}`,
			want:    DiskInfo{Disks: []Disk{}},
			wantErr: false,
		},
		{
			name:    "invalid json",
			data:    `{"disks":invalid}`,
			want:    DiskInfo{},
			wantErr: true,
		},
		{
			name:    "empty string",
			data:    ``,
			want:    DiskInfo{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := UnmarshalDiskInfo([]byte(tt.data))
			if (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalDiskInfo() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if len(got.Disks) != len(tt.want.Disks) {
					t.Errorf("UnmarshalDiskInfo() got %d disks, want %d", len(got.Disks), len(tt.want.Disks))
					return
				}
				for i := range got.Disks {
					if got.Disks[i] != tt.want.Disks[i] {
						t.Errorf("UnmarshalDiskInfo() disk[%d] = %v, want %v", i, got.Disks[i], tt.want.Disks[i])
					}
				}
			}
		})
	}
}

func TestHarvesterConfigJSON(t *testing.T) {
	hc := HarvesterConfig{
		TypeMeta: metav1.TypeMeta{
			Kind:       "HarvesterConfig",
			APIVersion: "rke-machine-config.cattle.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-config",
			Namespace: "fleet-default",
		},
		VMNamespace: "test-namespace",
		CPUcount:    "4",
		MemorySize:  "8",
		DiskSize:    "40",
		ImageName:   "ubuntu-20.04",
		SSHUser:     "ubuntu",
	}

	data, err := json.Marshal(hc)
	if err != nil {
		t.Fatalf("Failed to marshal HarvesterConfig: %v", err)
	}

	var decoded HarvesterConfig
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal HarvesterConfig: %v", err)
	}

	if decoded.Name != hc.Name {
		t.Errorf("Name mismatch: got %s, want %s", decoded.Name, hc.Name)
	}
	if decoded.VMNamespace != hc.VMNamespace {
		t.Errorf("VMNamespace mismatch: got %s, want %s", decoded.VMNamespace, hc.VMNamespace)
	}
	if decoded.CPUcount != hc.CPUcount {
		t.Errorf("CPUcount mismatch: got %s, want %s", decoded.CPUcount, hc.CPUcount)
	}
}
