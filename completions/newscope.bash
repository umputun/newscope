# bash completion for newscope (generated via go-flags)
_newscope() {
    local args=("${COMP_WORDS[@]:1:$COMP_CWORD}")
    mapfile -t COMPREPLY < <(GO_FLAGS_COMPLETION=1 "${COMP_WORDS[0]}" "${args[@]}" 2>/dev/null)
    return 0
}
complete -o default -F _newscope newscope
