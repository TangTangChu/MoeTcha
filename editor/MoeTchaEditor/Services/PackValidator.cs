using MoeTchaEditor.Models;
using SkiaSharp;

namespace MoeTchaEditor.Services;

public static class PackValidator
{
    public static IReadOnlyList<string> Validate(EditorPack pack)
    {
        var errors = new List<string>();
        if (pack == null)
        {
            errors.Add("素材包为空");
            return errors;
        }

        pack.Normalize();

        if (string.IsNullOrWhiteSpace(pack.PackName))
            errors.Add("包名不能为空");

        ValidateTags(pack, errors);
        ValidateGridConfig(pack, errors);

        var usedStems = new HashSet<string>(StringComparer.OrdinalIgnoreCase);
        ValidateGridImages(pack, errors, usedStems);
        ValidateClickImages(pack, errors, usedStems);

        return errors;
    }

    private static void ValidateTags(EditorPack pack, List<string> errors)
    {
        foreach (var pair in pack.TagDefs)
        {
            if (string.IsNullOrWhiteSpace(pair.Key))
                errors.Add("标签 key 不能为空");
            if (pair.Value == null || string.IsNullOrWhiteSpace(pair.Value.Name))
                errors.Add($"标签「{pair.Key}」的显示名不能为空");

            if (pair.Value?.Similar.Any(s => string.Equals(s, pair.Key, StringComparison.Ordinal)) == true)
                errors.Add($"标签「{pair.Key}」不能把自己列为相似标签");
            if (pair.Value?.Similar.Any(string.IsNullOrWhiteSpace) == true)
                errors.Add($"标签「{pair.Key}」包含空的相似标签");
            if (pair.Value?.Similar.Distinct(StringComparer.Ordinal).Count() != pair.Value?.Similar.Count)
                errors.Add($"标签「{pair.Key}」包含重复的相似标签");
            if (pair.Value?.Similar.Any(s => !pack.TagDefs.ContainsKey(s)) == true)
                errors.Add($"标签「{pair.Key}」引用了不存在的相似标签");
        }
    }

    private static void ValidateGridConfig(EditorPack pack, List<string> errors)
    {
        var grid = pack.Grid;
        if (grid.Size < 4)
            errors.Add("Grid 图片数至少为 4");
        if (grid.CorrectMin < 1)
            errors.Add("Grid 最少正确数至少为 1");
        if (grid.CorrectMax < grid.CorrectMin)
            errors.Add("Grid 最多正确数不能小于最少正确数");
        if (grid.CorrectMax >= grid.Size)
            errors.Add("Grid 最多正确数必须小于图片总数");
        if (!string.IsNullOrWhiteSpace(grid.Question)
            && !grid.Question.Contains("{tag}", StringComparison.Ordinal))
            errors.Add("Grid 问题模板必须包含 {tag}");
        if (!string.IsNullOrWhiteSpace(pack.Click.Question)
            && !pack.Click.Question.Contains("{tag}", StringComparison.Ordinal))
            errors.Add("Click 问题模板必须包含 {tag}");
    }

    private static void ValidateGridImages(
        EditorPack pack,
        List<string> errors,
        HashSet<string> usedStems)
    {
        for (var i = 0; i < pack.GridImages.Count; i++)
        {
            var image = pack.GridImages[i];
            if (image == null)
            {
                errors.Add($"Grid 图片 #{i + 1} 为空");
                continue;
            }

            var safeFile = ValidateFile(pack, image.File, $"Grid 图片 #{i + 1}", errors, usedStems);
            if (safeFile && FileExists(pack, image.File) && TryGetDimensions(pack, image.File) == null)
                errors.Add($"Grid 图片「{image.File}」无法解码");
            if (image.Tags.Count == 0 || image.Tags.All(string.IsNullOrWhiteSpace))
                errors.Add($"Grid 图片「{image.File}」至少需要一个标签");
            if (image.Tags.Any(string.IsNullOrWhiteSpace))
                errors.Add($"Grid 图片「{image.File}」包含空标签");
            if (image.Tags.Distinct(StringComparer.Ordinal).Count() != image.Tags.Count)
                errors.Add($"Grid 图片「{image.File}」包含重复标签");
        }
    }

    private static void ValidateClickImages(
        EditorPack pack,
        List<string> errors,
        HashSet<string> usedStems)
    {
        for (var i = 0; i < pack.ClickImages.Count; i++)
        {
            var image = pack.ClickImages[i];
            if (image == null)
            {
                errors.Add($"Click 图片 #{i + 1} 为空");
                continue;
            }

            var safeFile = ValidateFile(pack, image.File, $"Click 图片 #{i + 1}", errors, usedStems);
            if (image.Regions.Count == 0)
                errors.Add($"Click 图片「{image.File}」至少需要一个区域");

            var dimensions = safeFile ? TryGetDimensions(pack, image.File) : null;
            if (safeFile && FileExists(pack, image.File) && dimensions == null)
                errors.Add($"Click 图片「{image.File}」无法解码");
            for (var ri = 0; ri < image.Regions.Count; ri++)
            {
                var region = image.Regions[ri];
                if (region == null)
                {
                    errors.Add($"Click 图片「{image.File}」的区域 #{ri + 1} 为空");
                    continue;
                }

                if (string.IsNullOrWhiteSpace(region.Tag))
                    errors.Add($"Click 图片「{image.File}」的区域 #{ri + 1} 标签不能为空");
                if (region.X < 0 || region.Y < 0 || region.Width <= 0 || region.Height <= 0)
                    errors.Add($"Click 图片「{image.File}」的区域 #{ri + 1} 矩形无效");

                if (dimensions is { } size &&
                    ((long)region.X + region.Width > size.Width || (long)region.Y + region.Height > size.Height))
                {
                    errors.Add($"Click 图片「{image.File}」的区域 #{ri + 1} 超出图片边界");
                }
            }
        }
    }

    private static bool ValidateFile(
        EditorPack pack,
        string file,
        string label,
        List<string> errors,
        HashSet<string> usedStems)
    {
        if (string.IsNullOrWhiteSpace(file))
        {
            errors.Add($"{label}文件名不能为空");
            return false;
        }

        var safeFileName = !Path.IsPathRooted(file)
                           && string.Equals(Path.GetFileName(file), file, StringComparison.Ordinal);
        if (!safeFileName)
            errors.Add($"{label}文件名必须位于素材包根目录");

        var stem = Path.GetFileNameWithoutExtension(file);
        if (string.IsNullOrWhiteSpace(stem))
            errors.Add($"{label}文件名无效");
        else if (!usedStems.Add(stem))
            errors.Add($"图片文件名冲突：{stem}");

        if (string.IsNullOrWhiteSpace(pack.PackDirectory))
            return safeFileName;

        if (!safeFileName) return false;

        var path = Path.Combine(pack.PackDirectory, file);
        if (!File.Exists(path))
            errors.Add($"{label}文件不存在：{file}");
        return true;
    }

    private static (int Width, int Height)? TryGetDimensions(EditorPack pack, string file)
    {
        if (string.IsNullOrWhiteSpace(pack.PackDirectory) || string.IsNullOrWhiteSpace(file))
            return null;
        if (Path.IsPathRooted(file) || !string.Equals(Path.GetFileName(file), file, StringComparison.Ordinal))
            return null;

        try
        {
            using var bitmap = SKBitmap.Decode(Path.Combine(pack.PackDirectory, file));
            return bitmap == null ? null : (bitmap.Width, bitmap.Height);
        }
        catch
        {
            return null;
        }
    }

    private static bool FileExists(EditorPack pack, string file)
        => !string.IsNullOrWhiteSpace(pack.PackDirectory)
           && !string.IsNullOrWhiteSpace(file)
           && string.Equals(Path.GetFileName(file), file, StringComparison.Ordinal)
           && File.Exists(Path.Combine(pack.PackDirectory!, file));
}
