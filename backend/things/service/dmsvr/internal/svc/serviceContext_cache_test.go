package svc

import (
	"testing"

	"gitee.com/ming-iot/share/caches"
	"gitee.com/ming-iot/things/share/devices"
)

func TestProductDraftCacheInvalidationSelectsOnlySameProductDevices(t *testing.T) {
	productADevice := caches.GenKeyStr(devices.Core{ProductID: "product-a", DeviceName: "device-1"})
	productBDevice := caches.GenKeyStr(devices.Core{ProductID: "product-b", DeviceName: "device-1"})
	if !deviceSchemaCacheBelongsToProduct(productADevice, "product-a") {
		t.Fatal("same-product device cache key was not selected")
	}
	if deviceSchemaCacheBelongsToProduct(productBDevice, "product-a") {
		t.Fatal("other-product device cache key was selected")
	}
	if deviceSchemaCacheBelongsToProduct("not-json", "product-a") {
		t.Fatal("malformed device cache key was selected")
	}
}
