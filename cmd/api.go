/*
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
	"html/template"
	"os"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/ghodss/yaml"
	"github.com/spf13/cobra"

	"github.com/kubeshop/kgw/spec"
	"github.com/kubeshop/kgw/templates"
)

var (
	apiTemplate *template.Template
	apiSpecPath string

	name      string
	namespace string
	apiSpec   string
)

// apiCmd represents the api command
var apiCmd = &cobra.Command{
	Use:   "api",
	Short: "Generate kusk gateway api resources from an OpenAPI spec",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {

		parsedApiSpec, err := spec.NewParser(openapi3.NewLoader()).Parse(apiSpecPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		marshalledApiSpec, err := yaml.Marshal(parsedApiSpec)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		apiSpec = string(marshalledApiSpec)

		if err := apiTemplate.Execute(os.Stdout, templates.APITemplateArgs{
			Name:      name,
			Namespace: namespace,
			Spec:      apiSpec,
		}); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(apiCmd)

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

	apiTemplate = template.Must(template.New("api").Parse(templates.APITemplate))
}
