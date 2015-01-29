// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/storage"
)

const (
	// Loop provider types
	LoopProviderType     = storage.ProviderType("loop")
	HostLoopProviderType = storage.ProviderType("hostloop")

	// OOTB Storage pools.
	LoopPool = "loop"

	// Config attributes
	LoopDataDir = "data-dir" // top level directory where loop devices are created.
	LoopSubDir  = "sub-dir"  // optional subdirectory for loop devices.
)

// loopProviders create volume sources which use loop devices.
type loopProvider struct {
	run RunCommandFn
}

var (
	_ storage.Provider = (*loopProvider)(nil)

	// Standardized errors
	NoLoopDeviceErr = errors.New("could not find any free loop device")
)

// ValidateConfig is defined on the Provider interface.
func (lp *loopProvider) ValidateConfig(providerConfig *storage.Config) error {
	dataDir, ok := providerConfig.ValueString(LoopDataDir)
	if !ok || dataDir == "" {
		return errors.New("no data directory specified")
	}
	return nil
}

// VolumeSource is defined on the Provider interface.
func (lp *loopProvider) VolumeSource(environConfig *config.Config, providerConfig *storage.Config) (storage.VolumeSource, error) {
	if err := lp.ValidateConfig(providerConfig); err != nil {
		return nil, err
	}
	dataDir, _ := providerConfig.ValueString(LoopDataDir)
	subDir, _ := providerConfig.ValueString(LoopSubDir)
	return &loopVolumeSource{
		dataDir,
		subDir,
		lp.run,
		make(map[string]blockDevicePlus),
	}, nil
}

func (lp *loopProvider) FilesystemSource(environConfig *config.Config, providerConfig *storage.Config) (storage.FilesystemSource, error) {
	return nil, errors.NotSupportedf("filesystems")
}

// blockDevicePlus contains a reference to the BlockDevice that most
// of storage works with in addition to other information that is
// useful to the internal implementation of loopVolumeSource.
type blockDevicePlus struct {
	BlockDevice   storage.BlockDevice
	BlockFilePath string
}

// loopVolumeSource provides common functionality to handle
// loop devices for rootfs and host loop volume sources.
type loopVolumeSource struct {
	dataDir            string
	subDir             string
	runCmd             RunCommandFn
	volIdToBlockDevice map[string]blockDevicePlus
}

type RunCommandFn func(cmd string, args ...string) (string, error)

var _ storage.VolumeSource = (*loopVolumeSource)(nil)

func (lvs *loopVolumeSource) rootDeviceDir() string {
	dirParts := []string{lvs.dataDir}
	if lvs.subDir != "" {
		dirParts = append(dirParts, lvs.subDir)
	}
	return filepath.Join(dirParts...)
}

func (lvs *loopVolumeSource) CreateVolumes(args []storage.VolumeParams) ([]storage.BlockDevice, error) {

	blockDevices := make([]storage.BlockDevice, 0, len(args))
	for _, arg := range args {

		nextDevNum, err := findAvailableDeviceNumber(lvs.runCmd)
		if err != nil {
			return nil, errors.Annotate(err, "could not find next available loop device")
		}

		// TODO(katco-): Pass in machine ID.
		id := providerId("", fmt.Sprintf("loop%d", nextDevNum))
		filePath := filepath.Join(lvs.rootDeviceDir(), id)
		// TODO(katco-): Provider should ensure this directory exists. Remove this.
		if _, err := lvs.runCmd("mkdir", "-p", lvs.rootDeviceDir()); err != nil {
			return nil, errors.Annotate(err, "could not create file path")
		}
		if err := createBlockFile(lvs.runCmd, filePath, arg.Size); err != nil {
			return nil, errors.Annotate(err, "could not create block file")
		}

		devicePath, err := attachToBlockDevice(lvs.runCmd, filePath)

		// If we ran out of loop devices, create another.
		if err == NoLoopDeviceErr {
			if err = createLoopDevice(lvs.runCmd, nextDevNum); err != nil {
				os.Remove(filePath)
				return nil, errors.Annotate(err, "could not create loop device")
			}
			devicePath, err = attachToBlockDevice(lvs.runCmd, filePath)
		}
		if err != nil {
			os.Remove(filePath)
			return nil, errors.Annotate(err, "could not create loop device")
		}

		deviceName := devicePath[len("/dev/"):]
		blockDevice := storage.BlockDevice{
			Name:       arg.Name,
			ProviderId: id,
			DeviceName: deviceName,
			Size:       arg.Size,
			InUse:      false,
		}
		blockDevices = append(blockDevices, blockDevice)
		lvs.volIdToBlockDevice[id] = blockDevicePlus{blockDevice, filePath}
	}

	return blockDevices, nil
}

func (lvs *loopVolumeSource) DescribeVolumes(volIds []string) ([]storage.BlockDevice, error) {

	blockDevices := make([]storage.BlockDevice, len(volIds))
	for idx, volId := range volIds {
		device, ok := lvs.volIdToBlockDevice[volId]
		if !ok {
			return nil, errors.New("could not find volume ID " + volId)
		}
		blockDevices[idx] = device.BlockDevice
	}
	return blockDevices, nil
}

func (lvs *loopVolumeSource) DestroyVolumes(volIds []string) error {
	for _, volId := range volIds {
		device, ok := lvs.volIdToBlockDevice[volId]
		if !ok {
			return errors.New("could not find volume ID " + volId)
		} else if err := detachBlockDevice(lvs.runCmd, device.BlockDevice.DeviceName); err != nil {
			return err
		} else if err := removeBlockFile(lvs.runCmd, device.BlockFilePath); err != nil {
			return err
		}
	}
	return nil
}

func (lvs *loopVolumeSource) ValidateVolumeParams(params storage.VolumeParams) error {
	//panic("not implemented")
	return nil
}

func (lvs *loopVolumeSource) AttachVolumes(volIds []string, instId []instance.Id) error {
	//panic("not implemented")
	return nil
}

func (lvs *loopVolumeSource) DetachVolumes(volIds []string, instId []instance.Id) error {
	//panic("not implemented")
	return nil
}

func createBlockFile(run RunCommandFn, filePath string, sizeInMb uint64) error {
	output, err := run(
		"dd",
		"if=/dev/zero",
		fmt.Sprintf("of=%s", filePath),
		"bs=1024",
		fmt.Sprintf("count=%d", sizeInMb*1024),
	)
	return errors.Annotate(err, output)
}

func attachToBlockDevice(run RunCommandFn, filePath string) (loopDeviceName string, _ error) {
	// -f automatically finds the first available loop-device.
	// --show returns the loop device chosen on stdout.
	stdout, err := run("losetup", "-f", "--show", filePath)
	if err != nil && strings.Contains(err.Error(), "could not find any free loop device") {
		return "", NoLoopDeviceErr
	}
	return strings.TrimSpace(stdout), err
}

func detachBlockDevice(run RunCommandFn, devicePath string) error {
	_, err := run("losetup", "-d", devicePath)
	return err
}

func removeBlockFile(run RunCommandFn, filePath string) error {
	_, err := run("rm", filePath)
	return err
}

func createLoopDevice(run RunCommandFn, deviceNum uint) error {
	_, err := run("mknod", "-m", "660",
		fmt.Sprintf("/dev/loop%d", deviceNum),
		"b", "7", fmt.Sprintf("%d", deviceNum),
	)
	return err
}

func findAvailableDeviceNumber(run RunCommandFn) (uint, error) {

	// -a lists all attached loop devices.
	output, err := run("losetup", "-a")
	if err != nil {
		return 0, err
	} else if output == "" {
		return 0, nil
	}

	// Output will look like: /dev/loop0: [0801]:30551017 (/foo)
	devNum, err := strconv.Atoi(strings.Split(output, ":")[0][9:])
	if err != nil {
		return 0, err
	}

	return uint(devNum), nil
}

func providerId(machineId, loopDeviceName string) string {
	// TODO(katco-): Construct a smarter ID utilizing parameters.
	uuid, err := utils.NewUUID()
	if err != nil {
		panic(err)
	}
	return uuid.String()
	// return fmt.Sprintf("%s-%s", machineId, loopDeviceName)
}
