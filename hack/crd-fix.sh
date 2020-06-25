#!/bin/bash
#
# A script to fix CRD generations

# Note: script depends on line numbers so if anything is changed in the CRD generation it needs to be adjusted.

SCRIPTDIR=$(cd "$(dirname "${BASH_SOURCE[0]}" )" && pwd)
sed -i.bak "67s/.*//" $SCRIPTDIR/../deploy/crds/ibmcloud.ibm.com_v1alpha1_accessgroup_Crd.yaml
sed -i.bak "52s/.*//" $SCRIPTDIR/../deploy/crds/ibmcloud.ibm.com_v1alpha1_accesspolicy_crd.yaml
sed -i.bak "52s/.*//" $SCRIPTDIR/../deploy/crds/ibmcloud.ibm.com_v1alpha1_customrole_crd.yaml

# remove the .bak as they create issues with the releases
rm $SCRIPTDIR/../config/crds/*.yaml.bak
