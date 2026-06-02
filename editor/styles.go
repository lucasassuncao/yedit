package editor

import "github.com/lucasassuncao/yedit/theme"

func renderHeader(title, file string, dirty bool, width int, th resolvedTheme) string {
	info := file
	if dirty {
		info = file + " ● modified"
	}
	return theme.RenderHeaderWith(title, info, "", width, th.colors)
}
