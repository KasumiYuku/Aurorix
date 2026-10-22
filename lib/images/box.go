package images

// Box 图片显示上限; 宽或高 <= 0 表示该方向不设限。
type Box struct {
	Width  int
	Height int
}

const maxSide = 1 << 22

// On 报告上限是否生效。
func (b Box) On() bool { return b.Width > 0 || b.Height > 0 }

// Limit 按宽顶到上限、高由比例算出再封顶来给尺寸: 只缩不放, 高被截断时比例会偏离原图。
func (b Box) Limit(w, h int) (int, int) {
	if !b.On() || w <= 0 || h <= 0 || w > maxSide || h > maxSide || b.Width > maxSide || b.Height > maxSide {
		return w, h
	}
	ow := w
	if b.Width > 0 && w > b.Width {
		ow = b.Width
	}
	oh := scaled(int64(h), int64(ow), int64(w))
	if b.Height > 0 && oh > b.Height {
		oh = b.Height
	}
	return ow, oh
}

// scaled 返回 a×num÷den 四舍五入后的结果, 落在 [1, maxSide]。
func scaled(a, num, den int64) int {
	v := (a*num + den/2) / den
	switch {
	case v < 1:
		return 1
	case v > maxSide:
		return maxSide
	}
	return int(v)
}
