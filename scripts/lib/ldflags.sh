LDFLAGS_FOR() {
    printf -- "-X 'main.version=%s' -X 'main.gitCommit=%s' -X 'github.com/carroarmato0/nextui-itchio-pak/internal/ui.appVersion=%s' -X 'github.com/carroarmato0/nextui-itchio-pak/internal/netstate.buildDate=%s'" \
        "$1" "$2" "$1" "${BUILD_DATE:-}"
}
