package types

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

const auto = "AUTO"

// VolumeBinding src:dst[:flags][:size][:read_iops:write_iops:read_bytes:write_bytes]
type VolumeBinding struct {
	Source      string
	Destination string
	Flags       string
	SizeInBytes int64
	ReadIOPS    int64
	WriteIOPS   int64
	ReadBytes   int64
	WriteBytes  int64
}

// NewVolumeBinding returns pointer of VolumeBinding
func NewVolumeBinding(rawVolume string) (_ *VolumeBinding, err error) {
	vb := &VolumeBinding{}

	parts := strings.Split(rawVolume, ":")
	switch len(parts) {
	case 8:
		if vb.ReadIOPS, err = strconv.ParseInt(parts[4], 10, 64); err != nil {
			return nil, errors.WithStack(err)
		}
		if vb.WriteIOPS, err = strconv.ParseInt(parts[5], 10, 64); err != nil {
			return nil, errors.WithStack(err)
		}
		if vb.ReadBytes, err = strconv.ParseInt(parts[6], 10, 64); err != nil {
			return nil, errors.WithStack(err)
		}
		if vb.WriteBytes, err = strconv.ParseInt(parts[7], 10, 64); err != nil {
			return nil, errors.WithStack(err)
		}
		fallthrough
	case 4:
		if vb.SizeInBytes, err = strconv.ParseInt(parts[3], 10, 64); err != nil {
			return nil, errors.WithStack(err)
		}
		fallthrough
	case 3:
		vb.Flags = parts[2]
		fallthrough
	case 2:
		vb.Source, vb.Destination = parts[0], parts[1]
	default:
		return nil, errors.WithStack(fmt.Errorf("invalid rawVolume: %v", rawVolume))
	}

	flagParts := strings.Split(vb.Flags, "")
	sort.Strings(flagParts)
	vb.Flags = strings.Join(flagParts, "")

	return vb, vb.Validate()
}

// Validate return error if invalid
func (vb VolumeBinding) Validate() error {
	if vb.Destination == "" {
		return errors.WithStack(errors.Errorf("invalid volume, dest must be provided: %v", vb))
	}
	if vb.RequireScheduleMonopoly() && vb.RequireScheduleUnlimitedQuota() {
		return errors.WithStack(errors.Errorf("invalid volume, monopoly volume must not be limited: %v", vb))
	}
	if !vb.ValidIOParameters() {
		return errors.WithStack(errors.Errorf("invalid io parameters: %v", vb))
	}
	return nil
}

// RequireSchedule returns true if volume binding requires schedule
func (vb VolumeBinding) RequireSchedule() bool {
	return strings.HasSuffix(vb.Source, auto)
}

// RequireScheduleUnlimitedQuota .
func (vb VolumeBinding) RequireScheduleUnlimitedQuota() bool {
	return vb.RequireSchedule() && vb.SizeInBytes == 0
}

// RequireScheduleMonopoly returns true if volume binding requires monopoly schedule
func (vb VolumeBinding) RequireScheduleMonopoly() bool {
	return vb.RequireSchedule() && strings.Contains(vb.Flags, "m")
}

// ValidIOParameters returns true if all io related parameters are valid
func (vb VolumeBinding) ValidIOParameters() bool {
	return vb.ReadIOPS >= 0 && vb.WriteIOPS >= 0 && vb.ReadBytes >= 0 && vb.WriteBytes >= 0
}

// ToString returns volume string
func (vb VolumeBinding) ToString(normalize bool) (volume string) {
	flags := vb.Flags
	if normalize {
		flags = strings.ReplaceAll(flags, "m", "")
	}

	if strings.Contains(flags, "o") {
		flags = strings.ReplaceAll(flags, "o", "")
		flags = strings.ReplaceAll(flags, "r", "ro")
		flags = strings.ReplaceAll(flags, "w", "wo")
	}

	switch {
	case vb.Flags == "" && vb.SizeInBytes == 0:
		volume = fmt.Sprintf("%s:%s", vb.Source, vb.Destination)
	case vb.ReadIOPS != 0 || vb.WriteIOPS != 0 || vb.ReadBytes != 0 || vb.WriteBytes != 0:
		volume = fmt.Sprintf("%s:%s:%s:%d:%d:%d:%d:%d", vb.Source, vb.Destination, flags, vb.SizeInBytes, vb.ReadIOPS, vb.WriteIOPS, vb.ReadBytes, vb.WriteBytes)
	default:
		volume = fmt.Sprintf("%s:%s:%s:%d", vb.Source, vb.Destination, flags, vb.SizeInBytes)
	}
	return volume
}

// VolumeBindings is a collection of VolumeBinding
type VolumeBindings []*VolumeBinding

// NewVolumeBindings return VolumeBindings of reference type
func NewVolumeBindings(volumes []string) (volumeBindings VolumeBindings, err error) {
	for _, vb := range volumes {
		volumeBinding, err := NewVolumeBinding(vb)
		if err != nil {
			return nil, err
		}
		volumeBindings = append(volumeBindings, volumeBinding)
	}
	return
}

// ToStringSlice converts VolumeBindings into string slice
func (vbs VolumeBindings) ToStringSlice(sorted, normalize bool) (volumes []string) {
	if sorted {
		sort.Slice(vbs, func(i, j int) bool { return vbs[i].ToString(false) < vbs[j].ToString(false) })
	}
	for _, vb := range vbs {
		volumes = append(volumes, vb.ToString(normalize))
	}
	return
}

// UnmarshalJSON is used for encoding/json.Unmarshal
func (vbs *VolumeBindings) UnmarshalJSON(b []byte) (err error) {
	volumes := []string{}
	if err = json.Unmarshal(b, &volumes); err != nil {
		return errors.WithStack(err)
	}
	*vbs, err = NewVolumeBindings(volumes)
	return
}

// MarshalJSON is used for encoding/json.Marshal
func (vbs VolumeBindings) MarshalJSON() ([]byte, error) {
	volumes := []string{}
	for _, vb := range vbs {
		volumes = append(volumes, vb.ToString(false))
	}
	bs, err := json.Marshal(volumes)
	return bs, errors.WithStack(err)
}

// ApplyPlan creates new VolumeBindings according to volume plan
func (vbs VolumeBindings) ApplyPlan(plan VolumePlan) (res VolumeBindings) {
	for _, vb := range vbs {
		tmp := *vb
		newVb := &tmp
		if vmap, _ := plan.GetVolumeMap(vb); vmap != nil {
			newVb.Source = vmap.GetResourceID()
		}
		res = append(res, newVb)
	}
	return
}

// Divide .
func (vbs VolumeBindings) Divide() (soft VolumeBindings, hard VolumeBindings) {
	for _, vb := range vbs {
		if strings.HasSuffix(vb.Source, auto) {
			soft = append(soft, vb)
		} else {
			hard = append(hard, vb)
		}
	}
	return
}

// IsEqual return true is two VolumeBindings have the same value
func (vbs VolumeBindings) IsEqual(vbs2 VolumeBindings) bool {
	return reflect.DeepEqual(vbs.ToStringSlice(true, false), vbs2.ToStringSlice(true, false))
}

// TotalSize .
func (vbs VolumeBindings) TotalSize() (total int64) {
	for _, vb := range vbs {
		total += vb.SizeInBytes
	}
	return
}

// MergeVolumeBindings combines two VolumeBindings
func MergeVolumeBindings(vbs1 VolumeBindings, vbs2 ...VolumeBindings) (vbs VolumeBindings) {
	sizeMap := make(map[[3]string][]int64) // {["AUTO", "/data", "rw"]: [100, 0, 0, 0, 0]}
	for _, vbs := range append(vbs2, vbs1) {
		for _, vb := range vbs {
			key := [3]string{vb.Source, vb.Destination, vb.Flags}
			if _, ok := sizeMap[key]; !ok || sizeMap[key] == nil {
				sizeMap[key] = []int64{vb.SizeInBytes, vb.ReadIOPS, vb.WriteIOPS, vb.ReadBytes, vb.WriteBytes}
			} else {
				sizeMap[key][0] += vb.SizeInBytes
				sizeMap[key][1] += vb.ReadIOPS
				sizeMap[key][2] += vb.WriteIOPS
				sizeMap[key][3] += vb.ReadBytes
				sizeMap[key][4] += vb.WriteBytes
			}
		}
	}

	for key, para := range sizeMap {
		if para[0] < 0 {
			continue
		}
		vbs = append(vbs, &VolumeBinding{
			Source:      key[0],
			Destination: key[1],
			Flags:       key[2],
			SizeInBytes: para[0],
			ReadIOPS:    para[1],
			WriteIOPS:   para[2],
			ReadBytes:   para[3],
			WriteBytes:  para[4],
		})
	}
	return
}
