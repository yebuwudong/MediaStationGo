package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	"image/jpeg"
	"image/png"
	"math"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/gen2brain/webp"
	_ "golang.org/x/image/bmp"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/vp8l"
	xwebp "golang.org/x/image/webp"
)

const maxImageVariantInputBytes = 32 << 20

const imageVariantFFmpegTimeout = 4 * time.Second

var errImageVariantDecode = errors.New("image variant source is not decodable")

type imageVariantFallbackFunc func(context.Context, []byte, imageVariantOptions) ([]byte, string, error)

type imageVariantOptions struct {
	maxWidth   int
	maxHeight  int
	quality    int
	hasQuality bool
	fillWidth  int
	fillHeight int
	format     string
	err        error
}

// imageVariantFromRequest 解析图片变体参数。
//
// 设计原则：参数用于「客户端希望得到多大/什么格式的图」，属于渲染偏好，
// 不是访问准入条件。因此采用「归一 + 夹取」策略而非报错：
//
//   - 参数名大小写不敏感；同名参数重复（含大小写变体）时取首个有效值，
//     不再整体拒绝。真实客户端会同时携带 maxWidth 与 MaxWidth。
//   - 非数字、0、负值、超上限一律夹取到合法范围或忽略该参数，不再 400。
//     客户端常带 width=0 / quality=0 表示「不指定」。
//   - 仅当 fillWidth/fillHeight 只给一个时才视为无效并回退为不裁切。
//
// 历史行为（重复参数或越界即 400）会让部分客户端的图片请求整体失败。
func imageVariantFromRequest(r *http.Request) imageVariantOptions {
	o := imageVariantOptions{}
	if r == nil || r.URL == nil {
		return o
	}
	// 先按规范化后的参数名归组，避免 map 遍历顺序影响「取首值」的结果。
	grouped := make(map[string][]string)
	for name, values := range r.URL.Query() {
		name = strings.ToLower(name)
		switch name {
		case "width", "maxwidth", "height", "maxheight", "fillwidth", "fillheight", "quality", "format":
		default:
			continue
		}
		grouped[name] = append(grouped[name], values...)
	}
	// 固定顺序处理，保证结果确定：数值参数先规范化，format 最后覆盖。
	for _, name := range []string{"width", "maxwidth", "height", "maxheight", "fillwidth", "fillheight", "quality", "format"} {
		values, ok := grouped[name]
		if !ok || len(values) == 0 {
			continue
		}
		raw := strings.TrimSpace(values[0])
		if name == "format" {
			o.format = strings.ToLower(raw)
			if o.format == "jpg" {
				o.format = "jpeg"
			}
			// 不支持的格式按「未指定」处理，沿用默认输出格式。
			if o.format != "jpeg" && o.format != "webp" && o.format != "png" {
				o.format = ""
			}
			continue
		}
		value, err := strconv.Atoi(raw)
		if err != nil {
			// 无法解析的值视为未指定。
			continue
		}
		limit := 4096
		if name == "quality" {
			limit = 100
		}
		// 夹取到合法范围；0 与负值不再视为错误，而是「未指定」。
		if value > limit {
			value = limit
		}
		if value < 1 {
			continue
		}
		switch name {
		case "width", "maxwidth":
			if o.maxWidth == 0 || value < o.maxWidth {
				o.maxWidth = value
			}
		case "height", "maxheight":
			if o.maxHeight == 0 || value < o.maxHeight {
				o.maxHeight = value
			}
		case "fillwidth":
			o.fillWidth = value
		case "fillheight":
			o.fillHeight = value
		case "quality":
			o.quality, o.hasQuality = value, true
		}
	}
	// 只给一个 fill 参数时无法确定裁切比例，回退为不裁切而非报错。
	if (o.fillWidth == 0) != (o.fillHeight == 0) {
		o.fillWidth, o.fillHeight = 0, 0
	}
	return o
}

func (o imageVariantOptions) enabled() bool {
	return o.maxWidth > 0 || o.maxHeight > 0 || o.hasQuality || o.fillWidth > 0 || o.format != "" || o.err != nil
}

func (p *ImageProxy) serveImageVariant(w http.ResponseWriter, r *http.Request, key string, modTime time.Time, sourceSize int64, data []byte, contentType, cacheControl string, variant imageVariantOptions) error {
	variantKey, variantCachePath := p.imageVariantCachePaths(key, modTime, sourceSize, variant)
	if p.serveCachedImageVariantFile(w, r, variantKey, variantCachePath, cacheControl) {
		return nil
	}

	result := p.variantBuildGroup.DoChan(variantKey, func() (any, error) {
		p.variantBuildOnce.Do(func() { p.variantBuildSlots = make(chan struct{}, 2) })
		select {
		case p.variantBuildSlots <- struct{}{}:
		case <-r.Context().Done():
			return nil, r.Context().Err()
		}
		defer func() { <-p.variantBuildSlots }()
		out, outType, err := buildImageVariant(data, contentType, variant)
		if errors.Is(err, errImageVariantDecode) && p.variantFallback != nil {
			out, outType, err = p.variantFallback(r.Context(), data, variant)
		}
		if err != nil {
			return nil, err
		}
		if err := r.Context().Err(); err != nil {
			return nil, err
		}
		p.writeImageVariantCache(variantCachePath, out)
		return imageVariantResult{out, outType}, nil
	})
	var generated imageVariantResult
	select {
	case <-r.Context().Done():
		return r.Context().Err()
	case result := <-result:
		if result.Err != nil {
			return result.Err
		}
		generated = result.Val.(imageVariantResult)
	}
	out, outType := generated.data, generated.contentType
	w.Header().Del("Content-Length")
	w.Header().Set("Content-Type", outType)
	w.Header().Set("Cache-Control", cacheControl)
	w.Header().Set("ETag", imageVariantCacheETag(variantKey))
	http.ServeContent(w, r, variantKey, modTime, bytes.NewReader(out))
	return nil
}

type imageVariantResult struct {
	data        []byte
	contentType string
}

func buildImageVariant(data []byte, contentType string, variant imageVariantOptions) ([]byte, string, error) {
	if len(data) > maxImageVariantInputBytes {
		return nil, "", errors.New("image input limit exceeded")
	}
	if isTransparentPlaceholderData(data) {
		return data, "image/png", nil
	}
	var cfg image.Config
	var sourceFormat string
	var err error
	var webpSource webpVariantSource
	if len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		sourceFormat = "webp"
		webpSource, err = inspectWebPVariant(data)
		if err != nil {
			return nil, "", err
		}
		cfg = webpSource.config
	} else {
		cfg, sourceFormat, err = image.DecodeConfig(bytes.NewReader(data))
	}
	if err != nil {
		return nil, "", fmt.Errorf("%w: %v", errImageVariantDecode, err)
	}
	if !validVariantPixels(cfg.Width, cfg.Height) {
		return nil, "", errors.New("image pixel limit exceeded")
	}
	orientation, animated := variantMetadata(data, sourceFormat)
	if animated || sourceFormat == "gif" || webpSource.animated {
		return data, normalizedImageContentType(contentType, data), nil
	}
	var img image.Image
	if sourceFormat == "webp" {
		// Decode VP8L directly: the x/image WebP wrapper rejects valid VP8X
		// alpha + VP8L containers, while libwebp's old wrapper subsamples RGB.
		if webpSource.lossless != nil {
			img, err = vp8l.Decode(bytes.NewReader(webpSource.lossless))
		} else {
			img, err = xwebp.Decode(bytes.NewReader(data[:webpSource.end]))
		}
	} else {
		img, _, err = image.Decode(bytes.NewReader(data))
	}
	if err != nil {
		return nil, "", fmt.Errorf("%w: %v", errImageVariantDecode, err)
	}
	img = orientVariantImage(img, orientation)
	bounds := imageVariantCropBounds(img.Bounds(), variant)
	targetW, targetH := imageVariantTargetSize(bounds.Dx(), bounds.Dy(), variant)
	if targetW == cfg.Width && targetH == cfg.Height && !variant.hasQuality && variant.format == "" && variant.fillWidth == 0 && orientation < 2 {
		return data, normalizedImageContentType(contentType, data), nil
	}
	dst := image.NewNRGBA(image.Rect(0, 0, targetW, targetH))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, bounds, xdraw.Src, nil)
	quality := variant.quality
	if quality <= 0 {
		quality = defaultImageVariantQuality
	}
	quality = clampImageQuality(quality)
	var buf bytes.Buffer
	outType := "image/jpeg"
	switch variant.format {
	case "webp":
		outType = "image/webp"
		err = webp.Encode(&buf, dst, webp.Options{Quality: quality, Method: 4})
	case "png":
		outType = "image/png"
		err = png.Encode(&buf, dst)
	default:
		background := image.NewRGBA(dst.Bounds())
		xdraw.Draw(background, background.Bounds(), image.NewUniform(color.White), image.Point{}, xdraw.Src)
		xdraw.Draw(background, background.Bounds(), dst, image.Point{}, xdraw.Over)
		err = jpeg.Encode(&buf, background, &jpeg.Options{Quality: quality})
	}
	if err != nil {
		return nil, "", fmt.Errorf("encode image variant: %w", err)
	}
	return buf.Bytes(), outType, nil
}

func imageVariantCropBounds(bounds image.Rectangle, variant imageVariantOptions) image.Rectangle {
	if variant.fillWidth <= 0 || variant.fillHeight <= 0 {
		return bounds
	}
	if int64(bounds.Dx())*int64(variant.fillHeight) > int64(bounds.Dy())*int64(variant.fillWidth) {
		width := max(1, int(int64(bounds.Dy())*int64(variant.fillWidth)/int64(variant.fillHeight)))
		bounds.Min.X += (bounds.Dx() - width) / 2
		bounds.Max.X = bounds.Min.X + width
	} else {
		height := max(1, int(int64(bounds.Dx())*int64(variant.fillHeight)/int64(variant.fillWidth)))
		bounds.Min.Y += (bounds.Dy() - height) / 2
		bounds.Max.Y = bounds.Min.Y + height
	}
	return bounds
}

func normalizedImageContentType(contentType string, data []byte) string {
	contentType = strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	if isImageContentType(contentType) {
		return contentType
	}
	return detectContentType(data)
}

func (p *ImageProxy) transcodeImageVariantWithFFmpeg(ctx context.Context, data []byte, variant imageVariantOptions) ([]byte, string, error) {
	configured := ""
	if p != nil && p.cfg != nil {
		configured = p.cfg.App.FFmpegPath
	}
	bin, err := resolveLocalExecutable(configured, "ffmpeg")
	if err != nil {
		return nil, "", fmt.Errorf("image variant ffmpeg unavailable: %w", err)
	}
	runCtx, cancel := context.WithTimeout(ctx, imageVariantFFmpegTimeout)
	defer cancel()

	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-threads", "1",
		"-f", "image2pipe",
		"-i", "pipe:0",
		"-an", "-sn", "-dn",
	}
	if filter := imageVariantFFmpegScaleFilter(variant); filter != "" && variant.fillWidth == 0 {
		args = append(args, "-vf", filter)
	}
	extended := variant.fillWidth > 0 || (variant.format != "" && variant.format != "jpeg")
	codec := "mjpeg"
	if extended {
		codec = "png"
	}
	args = append(args, "-frames:v", "1", "-q:v", strconv.Itoa(imageVariantFFmpegQuality(variant)), "-f", "image2pipe", "-vcodec", codec, "pipe:1")

	cmd := exec.CommandContext(runCtx, bin, args...) // #nosec G204 -- executable is resolved locally and all arguments are numeric constants derived from validated query parameters.
	cmd.Stdin = bytes.NewReader(data)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if runCtx.Err() != nil {
			return nil, "", fmt.Errorf("image variant ffmpeg timed out: %w", runCtx.Err())
		}
		return nil, "", fmt.Errorf("image variant ffmpeg failed: %w: %s", err, truncateImageVariantError(stderr.String()))
	}
	out := stdout.Bytes()
	if len(out) == 0 || len(out) > maxImageVariantInputBytes {
		return nil, "", fmt.Errorf("image variant ffmpeg returned invalid size %d", len(out))
	}
	if extended {
		return buildImageVariant(out, "image/png", variant)
	}
	if _, err := jpeg.Decode(bytes.NewReader(out)); err != nil {
		return nil, "", fmt.Errorf("image variant ffmpeg returned invalid jpeg: %w", err)
	}
	return append([]byte(nil), out...), "image/jpeg", nil
}

func imageVariantFFmpegScaleFilter(variant imageVariantOptions) string {
	switch {
	case variant.maxWidth > 0 && variant.maxHeight > 0:
		return fmt.Sprintf("scale=min(iw\\,%d):min(ih\\,%d):force_original_aspect_ratio=decrease", variant.maxWidth, variant.maxHeight)
	case variant.maxWidth > 0:
		return fmt.Sprintf("scale=min(iw\\,%d):-2", variant.maxWidth)
	case variant.maxHeight > 0:
		return fmt.Sprintf("scale=-2:min(ih\\,%d)", variant.maxHeight)
	default:
		return ""
	}
}

func imageVariantFFmpegQuality(variant imageVariantOptions) int {
	quality := variant.quality
	if quality <= 0 {
		quality = defaultImageVariantQuality
	}
	quality = clampImageQuality(quality)
	return 2 + int(math.Round(float64(95-quality)*8.0/60.0))
}

func truncateImageVariantError(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 512 {
		return value
	}
	return value[:512]
}

func imageVariantTargetSize(width, height int, variant imageVariantOptions) (int, int) {
	scale := math.Min(1, math.Min(4096/float64(width), 4096/float64(height)))
	if variant.fillWidth > 0 && variant.fillHeight > 0 {
		scale = math.Min(scale, math.Min(float64(variant.fillWidth)/float64(width), float64(variant.fillHeight)/float64(height)))
	}
	if variant.maxWidth > 0 && width > variant.maxWidth {
		scale = math.Min(scale, float64(variant.maxWidth)/float64(width))
	}
	if variant.maxHeight > 0 && height > variant.maxHeight {
		scale = math.Min(scale, float64(variant.maxHeight)/float64(height))
	}
	if scale >= 1 {
		return width, height
	}
	targetW := int(math.Round(float64(width) * scale))
	targetH := int(math.Round(float64(height) * scale))
	if targetW < 1 {
		targetW = 1
	}
	if targetH < 1 {
		targetH = 1
	}
	return targetW, targetH
}

func clampImageQuality(value int) int {
	if value < 35 {
		return 35
	}
	if value > 95 {
		return 95
	}
	return value
}

const defaultImageVariantQuality = 82
