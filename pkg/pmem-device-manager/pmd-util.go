package pmdmanager

import (
	"fmt"
	"os"
	"strconv"
	"time"

	pmemexec "github.com/intel/pmem-csi/pkg/pmem-exec"
	"k8s.io/klog"
	"k8s.io/kubernetes/pkg/volume/util/hostutil"
)

const (
	retryStatTimeout time.Duration = 100 * time.Millisecond
)

func ClearDevice(device *PmemDeviceInfo, flush bool) error {
	klog.V(4).Infof("ClearDevice: path: %v flush:%v", device.Path, flush)
	// by default, clear 4 kbytes to avoid recognizing file system by next volume seeing data area
	var blocks uint64 = 4
	if flush {
		// clear all data if "erase all" asked specifically
		blocks = 0
	}
	return FlushDevice(device, blocks)
}

func FlushDevice(dev *PmemDeviceInfo, blocks uint64) error {
	// erase data on block device.
	// zero number of blocks causes overwriting whole device with random data.
	// nonzero number of blocks clears blocks*1024 bytes.
	// Before action, check that dev.Path exists and is device
	fileinfo, err := os.Stat(dev.Path)
	if err != nil {
		klog.Errorf("FlushDevice: %s does not exist", dev.Path)
		return err
	}
	if (fileinfo.Mode() & os.ModeDevice) == 0 {
		klog.Errorf("FlushDevice: %s is not device", dev.Path)
		return fmt.Errorf("%s is not device", dev.Path)
	}
	devOpen, err := hostutil.NewHostUtil().DeviceOpened(dev.Path)
	if err != nil {
		return err
	}
	if devOpen {
		return fmt.Errorf("%s is in use", dev.Path)
	}
	if blocks == 0 {
		klog.V(5).Infof("Wiping entire device: %s", dev.Path)
		// use one iteration instead of shred's default=3 for speed
		if _, err := pmemexec.RunCommand("shred", "-n", "1", dev.Path); err != nil {
			return fmt.Errorf("device shred failure: %v", err.Error())
		}
	} else {
		klog.V(5).Infof("Zeroing %d 1k blocks at start of device: %s Size %v", blocks, dev.Path, dev.Size)
		of := "of=" + dev.Path
		// guard against writing more than volume size
		if blocks*1024 > dev.Size {
			blocks = dev.Size / 1024
		}
		count := "count=" + strconv.FormatUint(blocks, 10)
		if _, err := pmemexec.RunCommand("dd", "if=/dev/zero", of, "bs=1024", count); err != nil {
			return fmt.Errorf("device zeroing failure: %v", err.Error())
		}
	}
	return nil
}

func WaitDeviceAppears(dev *PmemDeviceInfo) error {
	for i := 0; i < 10; i++ {
		_, err := os.Stat(dev.Path)
		if err == nil {
			return nil
		} else {
			klog.Warningf("WaitDeviceAppears[%d]: %s does not exist, sleep %v and retry",
				i, dev.Path, retryStatTimeout)
			time.Sleep(retryStatTimeout)
		}
	}
	return fmt.Errorf("device %s did not appear after multiple retries", dev.Path)
}
