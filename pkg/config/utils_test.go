// SPDX-FileCopyrightText: Copyright 2024 The Minder Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

func TestGetKeysWithNullValueFromYAML(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		yamlInput string
		want      []string
	}{
		{
			name: "Test with null values",
			yamlInput: `
key1: null
key2:
  subkey1: null
  subkey2: value
key3: [null, value]
`,
			want: []string{
				".key1",
				".key2.subkey1",
				".key3[0]",
			},
		},
		{
			name: "Test without null values",
			yamlInput: `
key1: value1
key2:
  subkey1: subvalue1
  subkey2: subvalue2
key3: [value1, value2]
`,
			want: []string{},
		},
		{
			name: "Test with highly nested null values",
			yamlInput: `
key1: value1
key2:
  subkey1: null
  subkey2: value2
  subkey3:
    subsubkey1: null
    subsubkey2: [value1, null, value2]
    subsubkey3:
      subsubsubkey1: [value1, value2, null]
      subsubsubkey2: null
key3: [value1, null, value2]
key4:
  subkey1: [value1, value2, null]
  subkey2:
    subsubkey1: null
    subsubkey2: [null, value1, value2]
    subsubkey3:
      subsubsubkey1: [value1, null, value2]
      subsubsubkey2: null
`,
			want: []string{
				".key2.subkey1",
				".key2.subkey3.subsubkey1",
				".key2.subkey3.subsubkey2[1]",
				".key2.subkey3.subsubkey3.subsubsubkey1[2]",
				".key2.subkey3.subsubkey3.subsubsubkey2",
				".key3[1]",
				".key4.subkey1[2]",
				".key4.subkey2.subsubkey1",
				".key4.subkey2.subsubkey2[0]",
				".key4.subkey2.subsubkey3.subsubsubkey1[1]",
				".key4.subkey2.subsubkey3.subsubsubkey2",
			},
		},
		{
			name: "Test with null values with integer, boolean, and null keys",
			yamlInput: `
key1: value1
true:
  null: null
  1: value2
  2:
    false: null
    3: [value1, null, value2]
    4:
      true: [value1, value2, null]
      5: null
key3: [value1, null, value2]
6:
  true: [value1, value2, null]
  7:
    false: null
    8: [null, value1, value2]
    9:
      true: [value1, null, value2]
      10: null
`,
			want: []string{
				".true.null",
				".true.2.false",
				".true.2.3[1]",
				".true.2.4.true[2]",
				".true.2.4.5",
				".key3[1]",
				".6.true[2]",
				".6.7.false",
				".6.7.8[0]",
				".6.7.9.true[1]",
				".6.7.9.10",
			},
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var data interface{}
			err := yaml.Unmarshal([]byte(test.yamlInput), &data)
			if err != nil {
				t.Fatalf("Error parsing YAML: %v", err)
			}

			got := GetKeysWithNullValueFromYAML(data, "")
			assert.ElementsMatchf(t, got, test.want, "GetKeysWithNullValueFromYAML() = %v, want %v", got, test.want)
		})
	}
}
