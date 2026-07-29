using System.Text.Encodings.Web;
using System.Text.Json;
using System.Text.Json.Serialization;
using MoeTchaEditor.Models;

namespace MoeTchaEditor.Services;

public static class PackSerializer
{
    private static readonly JsonSerializerOptions JsonOpts = new()
    {
        WriteIndented = true,
        PropertyNamingPolicy = JsonNamingPolicy.SnakeCaseLower,
        DefaultIgnoreCondition = JsonIgnoreCondition.WhenWritingDefault,
        Encoder = JavaScriptEncoder.UnsafeRelaxedJsonEscaping,
    };

    public static EditorPack Load(string metaJsonPath)
    {
        var json = File.ReadAllText(metaJsonPath);
        var pack = JsonSerializer.Deserialize<EditorPack>(json, JsonOpts)
            ?? throw new JsonException("元数据不是有效的素材包对象");
        pack.Normalize();
        pack.PackDirectory = Path.GetDirectoryName(Path.GetFullPath(metaJsonPath));
        pack.IsNew = false;
        return pack;
    }

    public static void Save(EditorPack pack)
    {
        var dir = pack.PackDirectory ?? throw new InvalidOperationException("PackDirectory 未设置");
        Directory.CreateDirectory(dir);

        var target = Path.Combine(dir, "meta.json");
        var temporary = Path.Combine(dir, $".meta.json.{Guid.NewGuid():N}.tmp");
        try
        {
            File.WriteAllText(temporary, JsonSerializer.Serialize(pack, JsonOpts));
            File.Move(temporary, target, true);
        }
        finally
        {
            try
            {
                if (File.Exists(temporary)) File.Delete(temporary);
            }
            catch
            {
                // 保存本身已经完成或正在报告原始异常，清理失败不应覆盖它。
            }
        }
    }

    public static EditorPack CreateNew(string dir)
    {
        var name = Path.GetFileName(Path.TrimEndingDirectorySeparator(dir));
        var pack = new EditorPack
        {
            PackName = string.IsNullOrWhiteSpace(name) ? "new_pack" : name,
            PackDirectory = dir,
            IsNew = true,
        };
        pack.Normalize();
        return pack;
    }
}
