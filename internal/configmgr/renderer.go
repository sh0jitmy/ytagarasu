// Copyright 2026 [Copyright Holder]
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Author: [YOUR_NAME]

package configmgr

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"text/template"
)

// Renderer parses and executes Go text/templates for application configuration files.
type Renderer struct {
	tmpl *template.Template
}

// NewRenderer creates a configuration template renderer with helper functions.
func NewRenderer(name, tmplStr string) (*Renderer, error) {
	funcMap := template.FuncMap{
		"env": func(key string, defVal ...string) string {
			if val := os.Getenv(key); val != "" {
				return val
			}
			if len(defVal) > 0 {
				return defVal[0]
			}
			return ""
		},
		"default": func(defVal, val string) string {
			if strings.TrimSpace(val) == "" {
				return defVal
			}
			return val
		},
		"lower": strings.ToLower,
		"upper": strings.ToUpper,
		"trim":  strings.TrimSpace,
	}

	t, err := template.New(name).Funcs(funcMap).Parse(tmplStr)
	if err != nil {
		return nil, fmt.Errorf("failed parsing config template '%s': %w", name, err)
	}

	return &Renderer{tmpl: t}, nil
}

// Render evaluates the template with provided data context.
func (r *Renderer) Render(data any) ([]byte, error) {
	var buf bytes.Buffer
	if err := r.tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("failed executing config template: %w", err)
	}
	return buf.Bytes(), nil
}
