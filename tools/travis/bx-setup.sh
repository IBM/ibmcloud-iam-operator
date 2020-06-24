#!/usr/bin/env bash
#
# Copyright 2019 IBM Corp. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#

set -e

BLUEMIX_RESOURCE_GROUP="${BLUEMIX_RESOURCE_GROUP:-Default}"

if [ -z $BLUEMIX_API_KEY ]; then
    echo "missing BLUEMIX_API_KEY. Aborting"
    exit 1
fi

if [ -z $BLUEMIX_ORG ]; then
    echo "missing BLUEMIX_ORG. Aborting"
    exit 1
fi

if [ -z $BLUEMIX_REGION ]; then
    echo "missing BLUEMIX_REGION. Aborting"
    exit 1
fi

ibmcloud login -a https://cloud.ibm.com --apikey ${BLUEMIX_API_KEY} -r $BLUEMIX_REGION -g $BLUEMIX_RESOURCE_GROUP

ibmcloud target -o $BLUEMIX_ORG


