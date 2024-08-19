#
# Copyright (c) 2020-2021 IOTech Ltd
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

FROM golang:1.23-alpine

LABEL license='SPDX-License-Identifier: Apache-2.0' \
    copyright='Copyright (c) 2019-2021: IoTech Ltd'

ARG ADD_BUILD_TAGS=""

RUN apk --no-cache upgrade \
    & apk add --update --no-cache dumb-init make git openssh

WORKDIR /iotdb-export

COPY go.mod vendor* ./
RUN [ ! -d "vendor" ] && go mod download all || echo "skipping..."

COPY . .

RUN make -e ADD_BUILD_TAGS=${ADD_BUILD_TAGS} build

EXPOSE 59790

ENTRYPOINT ["/iotdb-export"]
CMD ["--cp=consul://edgex-core-consul:8500", "--registry"]
