// SPDX-FileCopyrightText: 2020 SAP SE or an SAP affiliate company and Gardener contributors.
//
// SPDX-License-Identifier: Apache-2.0

package imagevector

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	cdv2 "github.com/gardener/component-spec/bindings-go/apis/v2"
	"github.com/gardener/component-spec/bindings-go/codec"
	"github.com/ghodss/yaml"
	"github.com/go-logr/logr"
	"github.com/mandelsoft/vfs/pkg/osfs"
	"github.com/mandelsoft/vfs/pkg/vfs"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/gardener/component-cli/pkg/commands/constants"
	"github.com/gardener/component-cli/pkg/imagevector"
	"github.com/gardener/component-cli/pkg/logger"
)

// GenerateOverwriteOptions defines the options that are used to generate a image vector from component descriptors
type GenerateOverwriteOptions struct {
	// ComponentDescriptorPath is the path to the component descriptor
	ComponentDescriptorPath string
	// ImageVectorPath defines the path to the image vector defined as yaml or json
	ImageVectorPath string
	// ComponentDescriptorsPath is a list of paths to additional component descriptors
	ComponentDescriptorsPath []string
}

// NewGenerateOverwriteCommand creates a command to add additional resources to a component descriptor.
func NewGenerateOverwriteCommand(ctx context.Context) *cobra.Command {
	opts := &GenerateOverwriteOptions{}
	cmd := &cobra.Command{
		Use:     "generate-overwrite",
		Aliases: []string{"go"},
		Short:   "Get parses a component descriptor and returns the defined image vector",
		Long: `
generate-overwrite parses images defined in a component descriptor and returns them as image vector.

Images can be defined in a component descriptor in 3 different ways:
1. as 'ociImage' resource: The image is defined a default resource of type 'ociImage' with a access of type 'ociRegistry'.
   It is expected that the resource contains the following labels to be identified as image vector image.
   The resulting image overwrite will contain the repository and the tag/digest from the access method.
<pre>

resources:
- name: pause-container
  version: "3.1"
  type: ociImage
  relation: external
  extraIdentity:
    "imagevector-gardener-cloud+tag": "3.1"
  labels:
  - name: imagevector.gardener.cloud/name
    value: pause-container
  - name: imagevector.gardener.cloud/repository
    value: gcr.io/google_containers/pause-amd64
  - name: imagevector.gardener.cloud/source-repository
    value: github.com/kubernetes/kubernetes/blob/master/build/pause/Dockerfile
  - name: imagevector.gardener.cloud/target-version
    value: "< 1.16"
  access:
    type: ociRegistry
    imageReference: gcr.io/google_containers/pause-amd64:3.1

</pre>

2. as component reference: The images are defined in a label "imagevector.gardener.cloud/images".
   The resulting image overwrite will contain all images defined in the images label.
   Their repository and tag/digest will be matched from the resources defined in the actual component's resources.

   Note: The images from the label are matched to the resources using their name and version. The original image reference do not exit anymore.

<pre>

componentReferences:
- name: cluster-autoscaler-abc
  componentName: github.com/gardener/autoscaler
  version: v0.10.1
  labels:
  - name: imagevector.gardener.cloud/images
    value:
      images:
      - name: cluster-autoscaler
        repository: eu.gcr.io/gardener-project/gardener/autoscaler/cluster-autoscaler
        tag: "v0.10.1"

</pre>

3. as generic images from the component descriptor labels.
   Generic images are images that do not directly result in a resource.
   They will be matched with another component descriptor that actually defines the images.
   The other component descriptor MUST have the "imagevector.gardener.cloud/name" label in order to be matched.

<pre>

meta:
  schemaVersion: 'v2'
component:
  labels:
  - name: imagevector.gardener.cloud/images
    value:
      images:
      - name: hyperkube
        repository: k8s.gcr.io/hyperkube
        targetVersion: "< 1.19"

</pre>

<pre>

meta:
  schemaVersion: 'v2'
component:
  resources:
  - name: hyperkube
    version: "v1.19.4"
    type: ociImage
    extraIdentity:
      "imagevector-gardener-cloud+tag": "v1.19.4"
    labels:
    - name: imagevector.gardener.cloud/name
      value: hyperkube
    - name: imagevector.gardener.cloud/repository
      value: k8s.gcr.io/hyperkube
    access:
	  type: ociRegistry
	  imageReference: my-registry/hyperkube:v1.19.4

</pre>

`,
		Run: func(cmd *cobra.Command, args []string) {
			if err := opts.Complete(args); err != nil {
				fmt.Println(err.Error())
				os.Exit(1)
			}

			if err := opts.Run(ctx, logger.Log, osfs.New()); err != nil {
				fmt.Println(err.Error())
				os.Exit(1)
			}
		},
	}

	opts.AddFlags(cmd.Flags())

	return cmd
}

func (o *GenerateOverwriteOptions) Run(ctx context.Context, log logr.Logger, fs vfs.FileSystem) error {
	data, err := vfs.ReadFile(fs, o.ComponentDescriptorPath)
	if err != nil {
		return fmt.Errorf("unable to read component descriptor from %q: %s", o.ComponentDescriptorPath, err.Error())
	}

	// add the input to the ctf format
	cd := &cdv2.ComponentDescriptor{}
	if err := codec.Decode(data, cd); err != nil {
		return fmt.Errorf("unable to decode component descriptor from %q: %s", o.ComponentDescriptorPath, err.Error())
	}

	// parse all given additional component descriptors
	cdList := &cdv2.ComponentDescriptorList{}
	for _, cdPath := range o.ComponentDescriptorsPath {
		data, err := vfs.ReadFile(fs, cdPath)
		if err != nil {
			return fmt.Errorf("unable to read component descriptor from %q: %s", cdPath, err.Error())
		}

		// add the input to the ctf format
		cd := cdv2.ComponentDescriptor{}
		if err := codec.Decode(data, &cd); err != nil {
			return fmt.Errorf("unable to decode component descriptor from %q: %s", cdPath, err.Error())
		}
		cdList.Components = append(cdList.Components, cd)
	}

	imageVector, err := imagevector.GenerateImageOverwrite(cd, cdList)
	if err != nil {
		return fmt.Errorf("unable to parse image vector: %s", err.Error())
	}

	data, err = yaml.Marshal(imageVector)
	if err != nil {
		return fmt.Errorf("unable to encode image vector: %w", err)
	}
	if len(o.ImageVectorPath) != 0 {
		if err := fs.MkdirAll(filepath.Dir(o.ImageVectorPath), os.ModePerm); err != nil {
			return fmt.Errorf("unable to create directories for %q: %s", o.ImageVectorPath, err.Error())
		}
		if err := vfs.WriteFile(fs, o.ImageVectorPath, data, 06444); err != nil {
			return fmt.Errorf("unable to write image vector: %w", err)
		}
		fmt.Printf("Successfully generated image vector from component descriptor")
	} else {
		fmt.Println(string(data))
	}
	return nil
}

func (o *GenerateOverwriteOptions) Complete(args []string) error {

	// default component path to env var
	if len(o.ComponentDescriptorPath) == 0 {
		o.ComponentDescriptorPath = filepath.Dir(os.Getenv(constants.ComponentDescriptorPathEnvName))
	}

	return o.validate()
}

func (o *GenerateOverwriteOptions) validate() error {
	if len(o.ComponentDescriptorPath) == 0 {
		return errors.New("component descriptor path must be provided")
	}
	return nil
}

func (o *GenerateOverwriteOptions) AddFlags(set *pflag.FlagSet) {
	set.StringVar(&o.ComponentDescriptorPath, "comp", "", "path to the component descriptor directory")
	set.StringArrayVar(&o.ComponentDescriptorsPath, "add-comp", []string{}, "path to the component descriptor directory")
	set.StringVar(&o.ImageVectorPath, "out", "", "The path to the image vector that will be written.")
}