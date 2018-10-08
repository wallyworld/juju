// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"fmt"
	"strings"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/schema"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/storage"
)

const (
	// K8s_ProviderType defines the Juju storage type which can be used
	// to provision storage on k8s models.
	K8s_ProviderType = storage.ProviderType(CAASProviderType)

	// K8s storage pool attributes.
	storageClass       = "storage-class"
	storageProvisioner = "storage-provisioner"
	storageLabel       = "storage-label"

	// K8s storage pool attribute default values.
	defaultStorageClass = "juju-unit-storage"
)

// StorageProviderTypes is defined on the storage.ProviderRegistry interface.
func (k *kubernetesClient) StorageProviderTypes() ([]storage.ProviderType, error) {
	return []storage.ProviderType{K8s_ProviderType}, nil
}

// StorageProvider is defined on the storage.ProviderRegistry interface.
func (k *kubernetesClient) StorageProvider(t storage.ProviderType) (storage.Provider, error) {
	if t == K8s_ProviderType {
		return &storageProvider{k}, nil
	}
	return nil, errors.NotFoundf("storage provider %q", t)
}

type storageProvider struct {
	client *kubernetesClient
}

var _ storage.Provider = (*storageProvider)(nil)

var storageConfigFields = schema.Fields{
	storageClass:       schema.String(),
	storageLabel:       schema.String(),
	storageProvisioner: schema.String(),
}

var storageConfigChecker = schema.FieldMap(
	storageConfigFields,
	schema.Defaults{
		storageClass:       schema.Omit,
		storageLabel:       schema.Omit,
		storageProvisioner: schema.Omit,
	},
)

type storageConfig struct {
	// storageClass defines a storage class
	// which will be created with the specified
	// provisioner and parameters if it doesn't
	// exist.
	storageClass string

	// storageProvisioner is the provisioner class to use.
	storageProvisioner string

	// parameters define attributes of the storage class.
	parameters map[string]string

	// existingStorageClass defines a storage class
	// which if present will be used, but if not
	// will fallback to looking for a storage class
	// based on the specified labels.
	existingStorageClass string

	// storageLabels define the labels used to
	// search for a storage class.
	storageLabels []string

	// reclaimPolicy defines the volume reclaim policy.
	reclaimPolicy core.PersistentVolumeReclaimPolicy
}

func newStorageConfig(attrs map[string]interface{}, defaultStorageClass string) (*storageConfig, error) {
	out, err := storageConfigChecker.Coerce(attrs, nil)
	if err != nil {
		return nil, errors.Annotate(err, "validating storage config")
	}
	coerced := out.(map[string]interface{})
	storageConfig := &storageConfig{
		existingStorageClass: defaultStorageClass,
	}
	if storageClassName, ok := coerced[storageClass].(string); ok {
		storageConfig.storageClass = storageClassName
	}
	if storageProvisioner, ok := coerced[storageProvisioner].(string); ok {
		storageConfig.storageProvisioner = storageProvisioner
	}
	if storageConfig.storageProvisioner != "" && storageConfig.storageClass == "" {
		return nil, errors.New("storage-class must be specified if storage-provisioner is specified")
	}
	// By default, we'll retain volumes used for charm storage.
	storageConfig.reclaimPolicy = core.PersistentVolumeReclaimRetain
	storageConfig.parameters = make(map[string]string)
	for k, v := range attrs {
		k = strings.TrimPrefix(k, "parameters.")
		storageConfig.parameters[k] = fmt.Sprintf("%v", v)
	}
	delete(storageConfig.parameters, storageClass)
	delete(storageConfig.parameters, storageLabel)
	delete(storageConfig.parameters, storageProvisioner)

	return storageConfig, nil
}

// ValidateConfig is defined on the storage.Provider interface.
func (g *storageProvider) ValidateConfig(cfg *storage.Config) error {
	_, err := newStorageConfig(cfg.Attrs(), defaultStorageClass)
	return errors.Trace(err)
}

// Supports is defined on the storage.Provider interface.
func (g *storageProvider) Supports(k storage.StorageKind) bool {
	return k == storage.StorageKindBlock
}

// Scope is defined on the storage.Provider interface.
func (g *storageProvider) Scope() storage.Scope {
	return storage.ScopeEnviron
}

// Dynamic is defined on the storage.Provider interface.
func (g *storageProvider) Dynamic() bool {
	return true
}

// Releasable is defined on the storage.Provider interface.
func (e *storageProvider) Releasable() bool {
	return true
}

// DefaultPools is defined on the storage.Provider interface.
func (g *storageProvider) DefaultPools() []*storage.Config {
	return nil
}

// VolumeSource is defined on the storage.Provider interface.
func (g *storageProvider) VolumeSource(cfg *storage.Config) (storage.VolumeSource, error) {
	return &volumeSource{
		client: g.client,
	}, nil
}

// FilesystemSource is defined on the storage.Provider interface.
func (g *storageProvider) FilesystemSource(providerConfig *storage.Config) (storage.FilesystemSource, error) {
	return nil, errors.NotSupportedf("filesystems")
}

type volumeSource struct {
	client *kubernetesClient
}

var _ storage.VolumeSource = (*volumeSource)(nil)

// CreateVolumes is specified on the storage.VolumeSource interface.
func (v *volumeSource) CreateVolumes(ctx context.ProviderCallContext, params []storage.VolumeParams) (_ []storage.CreateVolumesResult, err error) {
	// noop
	return nil, nil
}

// ListVolumes is specified on the storage.VolumeSource interface.
func (v *volumeSource) ListVolumes(ctx context.ProviderCallContext) ([]string, error) {
	pVolumes := v.client.CoreV1().PersistentVolumes()
	vols, err := pVolumes.List(v1.ListOptions{})
	if err != nil {
		return nil, errors.Trace(err)
	}
	volumeIds := make([]string, 0, len(vols.Items))
	for _, v := range vols.Items {
		volumeIds = append(volumeIds, v.Name)
	}
	return volumeIds, nil
}

// DescribeVolumes is specified on the storage.VolumeSource interface.
func (v *volumeSource) DescribeVolumes(ctx context.ProviderCallContext, volIds []string) ([]storage.DescribeVolumesResult, error) {
	pVolumes := v.client.CoreV1().PersistentVolumes()
	vols, err := pVolumes.List(v1.ListOptions{
		// TODO(caas) - filter on volumes for the current model
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	byId := make(map[string]core.PersistentVolume)
	for _, vol := range vols.Items {
		byId[vol.Name] = vol
	}
	results := make([]storage.DescribeVolumesResult, len(volIds))
	for i, volId := range volIds {
		vol, ok := byId[volId]
		if !ok {
			results[i].Error = errors.NotFoundf("%s", volId)
			continue
		}
		results[i].VolumeInfo = &storage.VolumeInfo{
			Size:       uint64(vol.Size()),
			VolumeId:   vol.Name,
			Persistent: vol.Spec.PersistentVolumeReclaimPolicy == core.PersistentVolumeReclaimRetain,
		}
	}
	return results, nil
}

// DestroyVolumes is specified on the storage.VolumeSource interface.
func (v *volumeSource) DestroyVolumes(ctx context.ProviderCallContext, volIds []string) ([]error, error) {
	logger.Debugf("destroy k8s volumes: %v", volIds)
	pVolumes := v.client.CoreV1().PersistentVolumes()
	return foreachVolume(volIds, func(volumeId string) error {
		return pVolumes.Delete(volumeId, &v1.DeleteOptions{
			PropagationPolicy: &defaultPropagationPolicy,
		})
	}), nil
}

// ReleaseVolumes is specified on the storage.VolumeSource interface.
func (v *volumeSource) ReleaseVolumes(ctx context.ProviderCallContext, volIds []string) ([]error, error) {
	// noop
	return make([]error, len(volIds)), nil
}

// ValidateVolumeParams is specified on the storage.VolumeSource interface.
func (v *volumeSource) ValidateVolumeParams(params storage.VolumeParams) error {
	// TODO(caas) - we need to validate params based on the underlying substrate
	return nil
}

// AttachVolumes is specified on the storage.VolumeSource interface.
func (v *volumeSource) AttachVolumes(ctx context.ProviderCallContext, attachParams []storage.VolumeAttachmentParams) ([]storage.AttachVolumesResult, error) {
	// noop
	return nil, nil
}

// DetachVolumes is specified on the storage.VolumeSource interface.
func (v *volumeSource) DetachVolumes(ctx context.ProviderCallContext, attachParams []storage.VolumeAttachmentParams) ([]error, error) {
	// noop
	return make([]error, len(attachParams)), nil
}

func foreachVolume(volumeIds []string, f func(string) error) []error {
	results := make([]error, len(volumeIds))
	var wg sync.WaitGroup
	for i, volumeId := range volumeIds {
		wg.Add(1)
		go func(i int, volumeId string) {
			defer wg.Done()
			results[i] = f(volumeId)
		}(i, volumeId)
	}
	wg.Wait()
	return results
}
