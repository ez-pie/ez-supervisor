#!/usr/bin/env bash

set -o errexit  # 同 set -e，脚本只要发生错误，就终止执行
set -o nounset  # 同 set -u，遇到不存在的变量会报错，并停止执行
set -o pipefail # 只要一个子命令失败，整个管道命令就失败，脚本就会终止执行（默认情况下只要最后一个子命令不失败，管道命令总是会执行成功）

SCRIPT_ROOT=$(dirname "${BASH_SOURCE[0]}")/..
CODEGEN_PKG=${CODEGEN_PKG:-$(
  cd "${SCRIPT_ROOT}"
  ls -d -1 ./vendor/k8s.io/code-generator 2>/dev/null || echo ../code-generator
)}

source "${CODEGEN_PKG}/kube_codegen.sh"

# generate the code with:
# --output-base    because this script should also be able to run inside the vendor dir of
#                  k8s.io/kubernetes. The output-base is needed for the generators to output into the vendor dir
#                  instead of the $GOPATH directly. For normal projects this can be dropped.

kube::codegen::gen_helpers \
  --input-pkg-root k8s.io/sample-controller/pkg/apis \
  --output-base "$(dirname "${BASH_SOURCE[0]}")/../../.." \
  --boilerplate "${SCRIPT_ROOT}/hack/boilerplate.go.txt"

kube::codegen::gen_client \
  --with-watch \
  --input-pkg-root k8s.io/sample-controller/pkg/apis \
  --output-pkg-root k8s.io/sample-controller/pkg/generated \
  --output-base "$(dirname "${BASH_SOURCE[0]}")/../../.." \
  --boilerplate "${SCRIPT_ROOT}/hack/boilerplate.go.txt"
