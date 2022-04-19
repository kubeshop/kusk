/*
The MIT License (MIT)

Copyright © 2021 Kubeshop

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.

*/
package cmd

import (
	"fmt"
	"os"
	"strings"
	"text/template"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/ghodss/yaml"
	"github.com/spf13/cobra"

	"github.com/kubeshop/kusk-gateway/pkg/options"
	"github.com/kubeshop/kusk-gateway/pkg/spec"
	"github.com/kubeshop/kusk/templates"
)

var (
	apiTemplate *template.Template
	apiSpecPath string

	name      string
	namespace string
	apiSpec   string

	serviceName      string
	serviceNamespace string
	servicePort      uint32
)

// generateCmd represents the generate command
var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate a Kusk Gateway API resource from your OpenAPI spec file",
	Long: `
	Generate accepts your OpenAPI spec file as input either as a local file or a URL pointing to your file
	and generates a Kusk Gateway compatible API resource that you can apply directly into your cluster.

	Configuration of the API resource is done via the x-kusk extension.

	If the OpenAPI spec doesn't have a top-level x-kusk annotation set, it will add them for you and set
	the upstream service, namespace and port to the flag values passed in respectively and set the rest of the settings to defaults.
	This is enough to get you started

	If the x-kusk extension is already present, it will override the the upstream service, namespace and port to the flag values passed in respectively
	and leave the rest of the settings as they are

	Sample usage

	No name specified
	kusk api generate -i spec.yaml

	In the above example, kusk will use the openapi spec info.title to generate a manifest name and leave the existing
	x-kusk extension settings
	
	No namespace specified
	kusk api generate -i spec.yaml --name httpbin-api --upstream.service httpbin --upstream.port 8080

	In the above example, as --namespace isn't defined, it will assume the default namespace.

	Namespace specified
	kusk api generate -i spec.yaml --name httpbin-api --upstream.service httpbin --upstream.namespace my-namespace --upstream.port 8080

	OpenAPI spec at URL
	kusk api generate \
			-i https://raw.githubusercontent.com/$ORG_OR_USER/$REPO/myspec.yaml \
			 --name httpbin-api \
			 --upstream.service httpbin \
			 --upstream.namespace my-namespace \
			 --upstream.port 8080
	
	This will fetch the OpenAPI document from the provided URL and generate a Kusk Gateway API resource
	`,
	Run: func(cmd *cobra.Command, args []string) {
		parsedApiSpec, err := spec.NewParser(openapi3.NewLoader()).Parse(apiSpecPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		if _, ok := parsedApiSpec.ExtensionProps.Extensions["x-kusk"]; !ok {
			parsedApiSpec.ExtensionProps.Extensions["x-kusk"] = options.Options{}
		}

		// if name flag is not defined, use the swagger doc title which is guarunteed to be there
		if name == "" {
			// kubernetes manifests cannot have . in the name so replace them
			name = strings.ReplaceAll(parsedApiSpec.Info.Title, ".", "-")
		}

		// override top level upstream service if undefined.
		if serviceName != "" && serviceNamespace != "" && servicePort != 0 {
			xKusk := parsedApiSpec.ExtensionProps.Extensions["x-kusk"].(options.Options)
			xKusk.Upstream = &options.UpstreamOptions{
				Service: &options.UpstreamService{
					Name:      serviceName,
					Namespace: serviceNamespace,
					Port:      servicePort,
				},
			}

			parsedApiSpec.ExtensionProps.Extensions["x-kusk"] = xKusk
		}

		if err := validateExtensionOptions(parsedApiSpec.ExtensionProps.Extensions["x-kusk"]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		if apiSpec, err = getAPISpecString(parsedApiSpec); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		if err := apiTemplate.Execute(os.Stdout, templates.APITemplateArgs{
			Name:      name,
			Namespace: namespace,
			Spec:      strings.Split(apiSpec, "\n"),
		}); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	},
}

func validateExtensionOptions(extension interface{}) error {
	b, err := yaml.Marshal(extension)
	if err != nil {
		return err
	}

	var o options.Options
	if err := yaml.Unmarshal(b, &o); err != nil {
		return err
	}

	o.FillDefaults()

	if err := o.Validate(); err != nil {
		return err
	}

	return nil
}

func getAPISpecString(apiSpec *openapi3.T) (string, error) {
	bApi, err := apiSpec.MarshalJSON()
	if err != nil {
		return "", err
	}

	yamlAPI, err := yaml.JSONToYAML(bApi)
	if err != nil {
		return "", nil
	}

	return string(yamlAPI), nil
}

func init() {
	apiCmd.AddCommand(generateCmd)

	generateCmd.Flags().StringVarP(
		&name,
		"name",
		"",
		"",
		"the name to give the API resource e.g. --name my-api",
	)

	generateCmd.Flags().StringVarP(
		&namespace,
		"namespace",
		"n",
		"default",
		"the namespace of the API resource e.g. --namespace my-namespace, -n my-namespace",
	)

	generateCmd.Flags().StringVarP(
		&apiSpecPath,
		"in",
		"i",
		"",
		"file path or URL to OpenAPI spec file to generate mappings from. e.g. --in apispec.yaml",
	)
	generateCmd.MarkFlagRequired("in")

	generateCmd.Flags().StringVarP(
		&serviceName,
		"upstream.service",
		"",
		"",
		"name of upstream service",
	)

	generateCmd.Flags().StringVarP(
		&serviceNamespace,
		"upstream.namespace",
		"",
		"default",
		"namespace of upstream service",
	)

	generateCmd.Flags().Uint32VarP(
		&servicePort,
		"upstream.port",
		"",
		80,
		"port of upstream service",
	)

	apiTemplate = template.Must(template.New("api").Parse(templates.APITemplate))
}
