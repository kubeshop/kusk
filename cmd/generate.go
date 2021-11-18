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

	"github.com/getkin/kin-openapi/jsoninfo"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/ghodss/yaml"
	"github.com/kubeshop/kusk-gateway/options"
	"github.com/kubeshop/kusk-gateway/spec"
	"github.com/spf13/cobra"

	"github.com/kubeshop/kgw/templates"
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
	Short: "Generate kusk gateway api resources from an OpenAPI spec",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		parsedApiSpec, err := spec.NewParser(openapi3.NewLoader()).Parse(apiSpecPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		if _, ok := parsedApiSpec.ExtensionProps.Extensions["x-kusk"]; !ok {
			parsedApiSpec.ExtensionProps.Extensions["x-kusk"] = options.Options{}
		}

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

		if err := jsoninfo.NewObjectEncoder().EncodeStructFieldsAndExtensions(&options.Options{}); err != nil {
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

	if err := o.FillDefaultsAndValidate(); err != nil {
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
		"the name to give the API resource",
	)
	generateCmd.MarkFlagRequired("name")

	generateCmd.Flags().StringVarP(
		&namespace,
		"namespace",
		"n",
		"default",
		"the namespace of the API resource",
	)

	generateCmd.Flags().StringVarP(
		&apiSpecPath,
		"in",
		"i",
		"",
		"file path to api spec file to generate mappings from. e.g. --in apispec.yaml",
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
