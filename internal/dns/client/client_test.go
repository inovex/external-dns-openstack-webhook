package client

import (
	"testing"

	golangsdk "github.com/opentelekomcloud/gophertelekomcloud"
	"github.com/opentelekomcloud/gophertelekomcloud/openstack"
)

func TestAuthOptionsFromCloudEnablesReauthForPasswordAuth(t *testing.T) {
	cloud := &openstack.Cloud{
		AuthType: openstack.AuthType("password"),
		AuthInfo: openstack.AuthInfo{
			AuthURL:           "https://iam.example.test/v3",
			Username:          "dns-user",
			Password:          "secret",
			ProjectName:       "project-name",
			UserDomainName:    "Default",
			ProjectDomainName: "Default",
		},
	}

	authOptions, err := authOptionsFromCloud(cloud)
	if err != nil {
		t.Fatalf("authOptionsFromCloud returned error: %v", err)
	}

	opts, ok := authOptions.(golangsdk.AuthOptions)
	if !ok {
		t.Fatalf("authOptionsFromCloud returned %T, want golangsdk.AuthOptions", authOptions)
	}
	if !opts.AllowReauth {
		t.Fatal("AllowReauth should be enabled for password auth")
	}
}
