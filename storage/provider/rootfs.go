// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/storage"
)

const (
	RootfsProviderType = storage.ProviderType("rootfs")
	RootfsPool         = "rootfs"

	// Config attributes
	RootfsStorageDir = "storage-dir"
)

// rootfsProviders create volume sources which use loop devices.
type rootfsProvider struct {
	run RunCommandFn
}

var (
	_ storage.Provider = (*rootfsProvider)(nil)
)

// ValidateConfig is defined on the Provider interface.
func (p *rootfsProvider) ValidateConfig(providerConfig *storage.Config) error {
	dir, ok := providerConfig.ValueString(RootfsStorageDir)
	if !ok || dir == "" {
		return errors.New("no storage directory specified")
	}
	return nil
}

func (p *rootfsProvider) VolumeSource(environConfig *config.Config, providerConfig *storage.Config) (storage.VolumeSource, error) {
	return nil, errors.NotSupportedf("volumes")
}

func (p *rootfsProvider) FilesystemSource(environConfig *config.Config, providerConfig *storage.Config) (storage.FilesystemSource, error) {
	if err := p.ValidateConfig(providerConfig); err != nil {
		return nil, err
	}
	storageDir, _ := providerConfig.ValueString(RootfsStorageDir)
	return &rootfsSource{p.run, storageDir}, nil
}

type rootfsSource struct {
	run        RunCommandFn
	storageDir string
}

var _ storage.FilesystemSource = (*rootfsSource)(nil)

func (s *rootfsSource) CreateFilesystems(args []storage.FilesystemParams) ([]storage.Filesystem, error) {
	filesystems := make([]storage.Filesystem, 0, len(args))
	for _, arg := range args {
		location := arg.Location
		if location == "" {
			location = filepath.Join(s.storageDir, arg.Name)
		}
		if _, err := os.Lstat(location); !os.IsNotExist(err) {
			// Ignore this request if the location already exists.
			continue
		}
		if err := os.MkdirAll(location, 0755); err != nil {
			return nil, errors.Annotate(err, "could not create directory")
		}
		dfOutput, err := s.run("df", "--output=size", location)
		if err != nil {
			os.Remove(location)
			return nil, errors.Annotate(err, "getting size")
		}
		lines := strings.SplitN(dfOutput, "\n", 2)
		blocks, err := strconv.ParseUint(strings.TrimSpace(lines[1]), 10, 64)
		if err != nil {
			os.Remove(location)
			return nil, errors.Annotate(err, "getting size")
		}
		sizeInMiB := blocks / 1024
		if sizeInMiB < arg.Size {
			os.Remove(location)
			return nil, errors.Annotatef(err, "filesystem is not big enough (%dM < %dM)", sizeInMiB, arg.Size)
		}

		fs := storage.Filesystem{
			Name:     arg.Name,
			Size:     sizeInMiB,
			Location: location,
		}
		filesystems = append(filesystems, fs)
	}
	return filesystems, nil
}
