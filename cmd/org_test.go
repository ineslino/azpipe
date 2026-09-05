package cmd

import "testing"

func TestOrganizationDestination(t *testing.T) {
	for _, input := range []string{"example", "https://dev.azure.com/example", "https://example.visualstudio.com"} {
		if _, err := validatedOrgURL(input); err != nil {
			t.Errorf("%s: %v", input, err)
		}
	}
	for _, input := range []string{"http://dev.azure.com/example", "https://evil.example/org", "https://dev.azure.com@example.com/org", "https://dev.azure.com/org?x=y", "https://dev.azure.com/org/other", "../other", "https://dev.azure.com:443/org"} {
		if _, err := validatedOrgURL(input); err == nil {
			t.Errorf("accepted %s", input)
		}
	}
}
