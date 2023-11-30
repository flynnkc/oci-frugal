# oci-frugal

## Pre-Requisites

- Oracle Cloud Infrastructure Account
- Administrator Privileges
- The OCI Python SDK v2.116.0

## Using the Script

### Options

__-c__ or __--compartment__ - (_Default: Root_) Specify a compartment OCID that the script should be run on. The script will only affect resources in the specified compartment.
__-e__ or __--exclude__ - (_Default: None_) Specify a compartment OCID that the script shoudl exclude from execution. The script will not run for resources in that compartment.
__-d__ or __--debug__ - (_Default: False_) Run the script in debug mode to collect detailed log information.
__-a__ or __--auth__ - (_Default: Profile_) Authentication mode values are [instance_principal, delegation_token]
__-p__ or __--profile__ - (_Default: DEFAULT_) Specify the profile name found in the OCI config file to use.
__-f__ or __--file__ - (_Default ~/.oci/config_) OCI config file location.
__-t__ or __--tag__ - (_Default: Schedule_) The tag namespace where resource schedules can be located.
