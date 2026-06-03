package diskutil

// ShrinkFSMountGuard bloqueia redução online do sistema de ficheiros raiz (/) antes de lvreduce/resize.
func ShrinkFSMountGuard() string {
	return "M=$(findmnt -sn -o TARGET --source \"$LV\" 2>/dev/null || true); " +
		"if [ \"$M\" = \"/\" ]; then " +
		"echo 'Redução do volume montado em / (raiz) não é suportada com o sistema em execução. " +
		"O ext4 precisa estar desmontado (recovery, live CD ou consola de manutenção).' >&2; exit 1; fi; "
}
