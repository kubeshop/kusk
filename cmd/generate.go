/*
Copyright © 2021 NAME HERE <EMAIL ADDRESS>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
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
	"github.com/spf13/cobra"

	"github.com/kubeshop/kgw/options"
	"github.com/kubeshop/kgw/spec"
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
			service := fmt.Sprintf("%s.%s.svc.cluster.local", serviceName, serviceNamespace)

			xKusk := parsedApiSpec.ExtensionProps.Extensions["x-kusk"].(options.Options)

			xKusk.Service.Name = service
			xKusk.Service.Port = servicePort

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

	apiCmd.Flags().StringVarP(
		&name,
		"name",
		"",
		"",
		"the name to give the API resource",
	)
	apiCmd.MarkFlagRequired("name")

	apiCmd.Flags().StringVarP(
		&namespace,
		"namespace",
		"n",
		"default",
		"the namespace of the API resource",
	)

	apiCmd.Flags().StringVarP(
		&apiSpecPath,
		"in",
		"i",
		"",
		"file path to api spec file to generate mappings from. e.g. --in apispec.yaml",
	)
	apiCmd.MarkFlagRequired("in")

	apiCmd.Flags().StringVarP(
		&serviceName,
		"upstream.service",
		"",
		"",
		"name of upstream service",
	)

	apiCmd.Flags().StringVarP(
		&serviceNamespace,
		"upstream.namespace",
		"",
		"default",
		"namespace of upstream service",
	)

	apiCmd.Flags().Uint32VarP(
		&servicePort,
		"upstream.port",
		"",
		80,
		"port of upstream service",
	)

	apiTemplate = template.Must(template.New("api").Parse(templates.APITemplate))
}
