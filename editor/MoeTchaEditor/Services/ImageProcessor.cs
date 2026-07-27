using SkiaSharp;

namespace MoeTchaEditor.Services;

public static class ImageProcessor
{
    private const int DefaultWebpQuality = 80;

    /// <summary>手动指定裁剪区域并转换为 WebP，返回输出文件名。</summary>
    public static string CropSquareAndWebp(
        string sourcePath,
        string outputDir,
        string baseFileName,
        int cropX,
        int cropY,
        int cropSize,
        int quality = DefaultWebpQuality)
    {
        using var bitmap = Decode(sourcePath);
        return CropSquareAndWebp(bitmap, outputDir, baseFileName, cropX, cropY, cropSize, quality);
    }

    /// <summary>转换为 WebP 格式（保持原始尺寸），返回输出文件名。</summary>
    public static string ToWebp(
        string sourcePath,
        string outputDir,
        string baseFileName,
        int quality = DefaultWebpQuality)
    {
        var outName = Path.ChangeExtension(Path.GetFileName(baseFileName), ".webp");
        var outPath = Path.Combine(outputDir, outName);
        Directory.CreateDirectory(outputDir);

        using var bitmap = Decode(sourcePath);
        using var image = SKImage.FromBitmap(bitmap);
        using var data = image.Encode(SKEncodedImageFormat.Webp, NormalizeQuality(quality))
            ?? throw new InvalidOperationException("WebP 编码失败");
        WriteAtomically(outPath, data);
        return outName;
    }

    private static string CropSquareAndWebp(
        SKBitmap bitmap,
        string outputDir,
        string baseFileName,
        int cropX,
        int cropY,
        int cropSize,
        int quality)
    {
        if (cropSize <= 0 || cropX < 0 || cropY < 0
            || cropX + cropSize > bitmap.Width || cropY + cropSize > bitmap.Height)
        {
            throw new ArgumentOutOfRangeException(nameof(cropSize), "裁剪区域超出图片边界");
        }

        var outName = Path.ChangeExtension(Path.GetFileName(baseFileName), ".webp");
        var outPath = Path.Combine(outputDir, outName);
        Directory.CreateDirectory(outputDir);

        using var image = SKImage.FromBitmap(bitmap);
        using var subset = image.Subset(SKRectI.Create(cropX, cropY, cropSize, cropSize))
            ?? throw new InvalidOperationException("无法创建裁剪区域");
        using var data = subset.Encode(SKEncodedImageFormat.Webp, NormalizeQuality(quality))
            ?? throw new InvalidOperationException("WebP 编码失败");
        WriteAtomically(outPath, data);
        return outName;
    }

    private static SKBitmap Decode(string sourcePath)
        => SKBitmap.Decode(sourcePath)
            ?? throw new InvalidOperationException($"无法解码图片：{sourcePath}");

    private static int NormalizeQuality(int quality)
        => quality is > 0 and <= 100 ? quality : DefaultWebpQuality;

    private static void WriteAtomically(string outputPath, SKData data)
    {
        var directory = Path.GetDirectoryName(outputPath)
            ?? throw new InvalidOperationException("输出目录无效");
        var temporary = Path.Combine(directory, $".{Path.GetFileName(outputPath)}.{Guid.NewGuid():N}.tmp");

        try
        {
            using (var stream = File.Create(temporary))
                data.SaveTo(stream);
            File.Move(temporary, outputPath, true);
        }
        finally
        {
            try
            {
                if (File.Exists(temporary)) File.Delete(temporary);
            }
            catch
            {
                // 保留原始编码/写入异常。
            }
        }
    }
}
