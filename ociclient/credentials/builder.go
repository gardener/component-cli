// SPDX-FileCopyrightText: 2021 SAP SE or an SAP affiliate company and Gardener contributors.
//
// SPDX-License-Identifier: Apache-2.0

package credentials

import (
	"bytes"
	"fmt"
	"path/filepath"

	"github.com/docker/cli/cli/config"
	dockerconfig "github.com/docker/cli/cli/config"
	"github.com/docker/cli/cli/config/credentials"
	dockerconfigtypes "github.com/docker/cli/cli/config/types"
	"github.com/go-logr/logr"
	"github.com/go-logr/logr/testing"
	"github.com/mandelsoft/vfs/pkg/osfs"
	"github.com/mandelsoft/vfs/pkg/vfs"
	corev1 "k8s.io/api/core/v1"
)

// KeyringBuilder is a builder to create and fill a keyring from different sources
type KeyringBuilder struct {
	log         logr.Logger
	fs          vfs.FileSystem
	pullSecrets []corev1.Secret
	configFiles []string

	disableDefaultConfig bool
}

// NewBuilder creates a new keyring builder
func NewBuilder(log logr.Logger) *KeyringBuilder {
	return &KeyringBuilder{
		log: log,
	}
}

// applyDefaults sets the builder defaults for undefined options
func (b *KeyringBuilder) applyDefaults() {
	if b.fs == nil {
		b.fs = osfs.New()
	}

	if !b.disableDefaultConfig {
		// add docker default config to config files
		defaultDockerConfigFile := filepath.Join(config.Dir(), config.ConfigFileName)

		// only add default if the file exists
		if _, err := b.fs.Stat(defaultDockerConfigFile); err == nil {
			b.configFiles = append(b.configFiles, defaultDockerConfigFile)
		}
	}
}

// DisableDefaultConfig disables the read from the default docker config on the system
func (b *KeyringBuilder) DisableDefaultConfig() *KeyringBuilder {
	b.disableDefaultConfig = true
	return b
}

// WithFS defines the filesystem that should be used to read data
func (b *KeyringBuilder) WithFS(fs vfs.FileSystem) *KeyringBuilder {
	b.fs = fs
	return b
}

// FromPullSecrets adds k8s secrets resources that contain pull secrets.
func (b *KeyringBuilder) FromPullSecrets(secrets ...corev1.Secret) *KeyringBuilder {
	b.pullSecrets = secrets
	return b
}

// FromConfigFiles adds file paths to docker config definitions
func (b *KeyringBuilder) FromConfigFiles(files ...string) *KeyringBuilder {
	b.configFiles = files
	return b
}

// Build creates a new oci registry keyring from the configured secrets.
func (b *KeyringBuilder) Build() (*GeneralOciKeyring, error) {
	b.applyDefaults()
	store := New()
	for _, secret := range b.pullSecrets {
		if secret.Type != corev1.SecretTypeDockerConfigJson {
			continue
		}
		dockerConfigBytes, ok := secret.Data[corev1.DockerConfigJsonKey]
		if !ok {
			continue
		}

		dockerConfig, err := dockerconfig.LoadFromReader(bytes.NewBuffer(dockerConfigBytes))
		if err != nil {
			return nil, err
		}

		// currently only support the default credential store.
		credStore := dockerConfig.GetCredentialsStore("")
		if err := store.Add(credStore); err != nil {
			return nil, err
		}
	}

	for _, configFile := range b.configFiles {
		if len(configFile) == 0 {
			continue
		}
		dockerConfigBytes, err := vfs.ReadFile(b.fs, configFile)
		if err != nil {
			return nil, err
		}

		dockerConfig, err := dockerconfig.LoadFromReader(bytes.NewBuffer(dockerConfigBytes))
		if err != nil {
			return nil, err
		}

		for address, auth := range dockerConfig.AuthConfigs {
			if err := store.AddAuthConfig(address, auth); err != nil {
				return nil, fmt.Errorf("unable to add auth for %q to store: %w", address, err)
			}
			b.log.V(10).Info(fmt.Sprintf("added authentication for %q from default docker config", address))
		}

		// add native store for external program authentication
		for adr, hlp := range dockerConfig.CredentialHelpers {
			address, helper := adr, hlp // store in new vars to be available in the auth config getter
			nativeStore := credentials.NewNativeStore(dockerConfig, helper)
			err := store.AddAuthConfigGetter(address, func(_ string) (dockerconfigtypes.AuthConfig, error) {
				b.log.V(8).Info(fmt.Sprintf("use oci cred helper %q to get %q", helper, address))
				auth, err := nativeStore.Get(address)
				if err != nil {
					msg := fmt.Sprintf("unable to get oci authentication information from external credentials helper %q for %q: %s", helper, address, err.Error())
					b.log.V(4).Info(msg)
				}
				return auth, err
			})
			b.log.V(10).Info(fmt.Sprintf("added authentication for %q with credential helper %s", address, helper))
			if err != nil {
				return nil, err
			}
		}
	}

	return store, nil
}

// CreateOCIRegistryKeyringFromFilesystem creates a new OCI registry keyring from a given file system.
// DEPRECATED: Use the Configbuilder
func CreateOCIRegistryKeyringFromFilesystem(pullSecrets []corev1.Secret, configFiles []string, fs vfs.FileSystem) (*GeneralOciKeyring, error) {
	return NewBuilder(testing.NullLogger{}).WithFS(fs).FromConfigFiles(configFiles...).FromPullSecrets(pullSecrets...).Build()
}

// CreateOCIRegistryKeyring creates a new OCI registry keyring.
// DEPRECATED: Use the Configbuilder
func CreateOCIRegistryKeyring(pullSecrets []corev1.Secret, configFiles []string) (*GeneralOciKeyring, error) {
	return NewBuilder(testing.NullLogger{}).WithFS(osfs.New()).FromConfigFiles(configFiles...).FromPullSecrets(pullSecrets...).Build()
}