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

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/storage"
)

const (
	TmpfsProviderType = storage.ProviderType("tmpfs")
	TmpfsPool         = "tmpfs"

	// Config attributes
	TmpfsStorageDir = "storage-dir"
)

// tmpfsProviders create volume sources which use loop devices.
type tmpfsProvider struct {
	run RunCommandFn
}

var (
	_ storage.Provider = (*tmpfsProvider)(nil)
)

// ValidateConfig is defined on the Provider interface.
func (p *tmpfsProvider) ValidateConfig(providerConfig *storage.Config) error {
	dir, ok := providerConfig.ValueString(TmpfsStorageDir)
	if !ok || dir == "" {
		return errors.New("no storage directory specified")
	}
	return nil
}

func (p *tmpfsProvider) VolumeSource(environConfig *config.Config, providerConfig *storage.Config) (storage.VolumeSource, error) {
	return nil, errors.NotSupportedf("volumes")
}

func (p *tmpfsProvider) FilesystemSource(environConfig *config.Config, providerConfig *storage.Config) (storage.FilesystemSource, error) {
	if err := p.ValidateConfig(providerConfig); err != nil {
		return nil, err
	}
	storageDir, _ := providerConfig.ValueString(TmpfsStorageDir)
	return &tmpfsSource{p.run, storageDir}, nil
}

type tmpfsSource struct {
	run        RunCommandFn
	storageDir string
}

var _ storage.FilesystemSource = (*tmpfsSource)(nil)

func (s *tmpfsSource) CreateFilesystems(args []storage.FilesystemParams) ([]storage.Filesystem, error) {
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
		if _, err := s.run(
			"mount", "-t", "tmpfs", "none", location,
			"-o", fmt.Sprintf("size=%d", arg.Size*1024*1024),
		); err != nil {
			os.Remove(location)
			return nil, errors.Annotate(err, "cannot mount tmpfs")
		}
		sizeInMiB, err := calculateSize(s.run, location)
		if err != nil {
			os.Remove(location)
			return nil, errors.Annotate(err, "getting size")
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

func calculateSize(run RunCommandFn, location string) (uint64, error) {
	dfOutput, err := run("df", "--output=size", location)
	if err != nil {
		return 0, errors.Annotate(err, "getting size")
	}
	lines := strings.SplitN(dfOutput, "\n", 2)
	blocks, err := strconv.ParseUint(strings.TrimSpace(lines[1]), 10, 64)
	if err != nil {
		return 0, errors.Annotate(err, "getting size")
	}
	return blocks / 1024, nil
}
