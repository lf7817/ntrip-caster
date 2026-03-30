package mountpoint

import (
	"math"
	"testing"
	"time"

	"ntrip-caster/internal/rtcm"
)

func TestUpdateAntennaPosition(t *testing.T) {
	mp := NewMountPoint("TEST", "Test mount", "RTCM3", 64, 3*time.Second, 0)

	// 初始状态：无位置
	pos := mp.GetAntennaPosition()
	if pos != nil {
		t.Error("initial position should be nil")
	}

	// 第一次更新
	pos1 := &rtcm.AntennaPosition{
		Latitude:  39.9,
		Longitude: 116.4,
		Height:    50,
		UpdatedAt: time.Now().Unix(),
	}
	mp.UpdateAntennaPosition(pos1)

	got := mp.GetAntennaPosition()
	if got == nil {
		t.Fatal("position should be set after update")
	}
	if math.Abs(got.Latitude-pos1.Latitude) > 0.001 {
		t.Errorf("lat = %.4f, want %.4f", got.Latitude, pos1.Latitude)
	}

	// 快速更新（5秒内）应被防抖忽略
	pos2 := &rtcm.AntennaPosition{
		Latitude:  40.0,
		Longitude: 117.0,
		Height:    60,
		UpdatedAt: time.Now().Unix(),
	}
	mp.UpdateAntennaPosition(pos2)

	got2 := mp.GetAntennaPosition()
	// 位置不应变化（5秒防抖）
	if math.Abs(got2.Latitude-pos1.Latitude) > 0.001 {
		t.Errorf("position changed despite debounce: lat = %.4f, want %.4f", got2.Latitude, pos1.Latitude)
	}
}

func TestAntennaPositionDebounce(t *testing.T) {
	mp := NewMountPoint("TEST", "Test mount", "RTCM3", 64, 3*time.Second, 0)

	// 第一次更新
	pos1 := &rtcm.AntennaPosition{
		Latitude:  39.9,
		Longitude: 116.4,
		Height:    50,
		UpdatedAt: time.Now().Unix(),
	}
	mp.UpdateAntennaPosition(pos1)

	// 模拟防抖过期
	mp.SetAntennaPosLastUpdate(time.Now().Add(-6 * time.Second))

	// 第二次更新应该成功
	pos2 := &rtcm.AntennaPosition{
		Latitude:  40.0,
		Longitude: 117.0,
		Height:    60,
		UpdatedAt: time.Now().Unix(),
	}
	mp.UpdateAntennaPosition(pos2)

	got := mp.GetAntennaPosition()
	if got == nil {
		t.Fatal("position should be set")
	}
	if math.Abs(got.Latitude-pos2.Latitude) > 0.001 {
		t.Errorf("position not updated after debounce: lat = %.4f, want %.4f", got.Latitude, pos2.Latitude)
	}
}