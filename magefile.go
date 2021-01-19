// +build mage

package main

import (
	spartaMage "github.com/mweagle/Sparta/magefile"
)

// Build the service
func Build() error {
	return spartaMage.Build()
}

// Provision the service
func Provision(s3Bucket string) error {
	return spartaMage.Provision(s3Bucket)
}

func ProvisionDocker(s3Bucket string, dockerFile string) error {
	return spartaMage.ProvisionDocker(s3Bucket, dockerFile)
}

// Describe the stack by producing an HTML representation of the CloudFormation
// template
func Describe(s3Bucket string) error {
	return spartaMage.Describe(s3Bucket)
}

// Delete the service, iff it exists
func Delete() error {
	return spartaMage.Delete()
}

// Status report if the stack has been provisioned
func Status() error {
	return spartaMage.Status()
}

// Version information
func Version() error {
	return spartaMage.Version()
}

// Build, then upload, then create changeset and wait?
