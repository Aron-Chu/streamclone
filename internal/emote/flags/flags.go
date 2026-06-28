package flags

const (
	ZeroWidth = 1 << 0
	Animated  = 1 << 1

	// SevenTV API EmoteFlagsZeroWidth (structures/v3/type.emote.go).
	SevenTVZeroWidth = 1 << 8
)

func IsZeroWidth(f int) bool {
	return f&ZeroWidth != 0
}

func IsAnimated(f int) bool {
	return f&Animated != 0
}

func FromSevenTV(setItemFlags, dataFlags int) bool {
	return setItemFlags&SevenTVZeroWidth != 0 || dataFlags&SevenTVZeroWidth != 0
}

func Pack(zeroWidth, animated bool) int {
	f := 0
	if zeroWidth {
		f |= ZeroWidth
	}
	if animated {
		f |= Animated
	}
	return f
}
