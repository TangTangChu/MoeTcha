using System.Collections.Generic;
using System.Linq;
using MoeTchaEditor.Models;

namespace MoeTchaEditor.Services;

/// <summary>
/// 单个标签在素材包中的引用快照。不可变；每次重建索引时整体替换。
/// </summary>
public sealed class TagUsage
{
    public string Key { get; }
    public bool IsDefined { get; }

    public int GridCount { get; }
    public int ClickCount { get; }
    public int SimilarCount { get; }

    /// <summary>已定义但 Grid / Click / Similar 三处均未被引用。</summary>
    public bool IsUnused => IsDefined && GridCount == 0 && ClickCount == 0 && SimilarCount == 0;

    /// <summary>引用此标签的 Grid 图片文件名列表（已去重，按出现顺序）。</summary>
    public IReadOnlyList<string> GridFiles { get; }

    /// <summary>引用此标签的 Click 图片文件名列表（同一图多个 region 只记一次）。</summary>
    public IReadOnlyList<string> ClickFiles { get; }

    /// <summary>把此标签列为 similar 的其他标签 key 列表。</summary>
    public IReadOnlyList<string> SimilarFrom { get; }

    public TagUsage(
        string key,
        bool isDefined,
        int gridCount,
        int clickCount,
        int similarCount,
        IReadOnlyList<string> gridFiles,
        IReadOnlyList<string> clickFiles,
        IReadOnlyList<string> similarFrom)
    {
        Key = key;
        IsDefined = isDefined;
        GridCount = gridCount;
        ClickCount = clickCount;
        SimilarCount = similarCount;
        GridFiles = gridFiles;
        ClickFiles = clickFiles;
        SimilarFrom = similarFrom;
    }
}

/// <summary>
/// 维护 tag → 引用 的反向索引。每次 Pack 图变化后由调用方触发 Rebuild。
/// 数据量在编辑器规模下很小（百张图 × 几个 tag），整体重建为 O(n)，无需增量维护。
/// </summary>
public sealed class TagUsageIndex
{
    private readonly Dictionary<string, TagUsage> _map = new(StringComparer.Ordinal);
    private readonly List<string> _definedKeys = new();
    private readonly List<string> _danglingKeys = new();
    private readonly List<string> _unusedKeys = new();

    /// <summary>按 tag key 查询引用快照。未定义且未被引用的 key 返回 null。</summary>
    public TagUsage? Get(string? key)
        => key != null && _map.TryGetValue(key, out var u) ? u : null;

    /// <summary>该 key 是否在 TagDefs 中定义。</summary>
    public bool IsDefined(string? key)
        => key != null && _map.TryGetValue(key, out var u) && u.IsDefined;

    /// <summary>所有出现过的 tag key（已定义 + 图片中引用但未定义的悬空 tag），按字母序。</summary>
    public IReadOnlyList<string> AllKeys => _map.Keys
        .OrderBy(k => k, StringComparer.Ordinal)
        .ToList();

    /// <summary>仅在 TagDefs 中定义的 key，按字母序。</summary>
    public IReadOnlyList<string> DefinedKeys => _definedKeys;

    /// <summary>被图片引用但未在 TagDefs 中定义的悬空 tag，按字母序。</summary>
    public IReadOnlyList<string> DanglingKeys => _danglingKeys;

    /// <summary>已定义但完全未被引用的标签（Grid/Click/Similar 均为 0），按字母序。</summary>
    public IReadOnlyList<string> UnusedDefinedKeys => _unusedKeys;

    /// <summary>当前快照里所有 TagUsage（含悬空），按 key 字母序。</summary>
    public IEnumerable<TagUsage> All
        => _map.Keys.OrderBy(k => k, StringComparer.Ordinal).Select(k => _map[k]);

    /// <summary>重建索引。线程不安全，假定在 UI 线程调用。</summary>
    public void Rebuild(EditorPack pack)
    {
        _map.Clear();
        _definedKeys.Clear();
        _danglingKeys.Clear();
        _unusedKeys.Clear();

        if (pack == null) return;

        var gridFiles = new Dictionary<string, List<string>>(StringComparer.Ordinal);
        var clickFiles = new Dictionary<string, List<string>>(StringComparer.Ordinal);
        var similarFrom = new Dictionary<string, List<string>>(StringComparer.Ordinal);
        var definedSet = new HashSet<string>(StringComparer.Ordinal);

        foreach (var image in pack.GridImages)
        {
            if (image == null) continue;
            var file = image.File ?? "";
            var seenHere = new HashSet<string>(StringComparer.Ordinal);
            foreach (var tag in image.Tags ?? Enumerable.Empty<string>())
            {
                if (string.IsNullOrWhiteSpace(tag)) continue;
                var key = tag.Trim();
                if (!seenHere.Add(key)) continue;
                if (!gridFiles.TryGetValue(key, out var list))
                {
                    list = new List<string>();
                    gridFiles[key] = list;
                }
                list.Add(file);
            }
        }

        foreach (var image in pack.ClickImages)
        {
            if (image == null) continue;
            var file = image.File ?? "";
            var seenHere = new HashSet<string>(StringComparer.Ordinal);
            foreach (var region in image.Regions ?? Enumerable.Empty<RegionInfo>())
            {
                if (region == null) continue;
                var tag = region.Tag;
                if (string.IsNullOrWhiteSpace(tag)) continue;
                var key = tag.Trim();
                if (!seenHere.Add(key)) continue;
                if (!clickFiles.TryGetValue(key, out var list))
                {
                    list = new List<string>();
                    clickFiles[key] = list;
                }
                list.Add(file);
            }
        }

        foreach (var pair in pack.TagDefs ?? new())
        {
            if (pair.Value == null) continue;
            definedSet.Add(pair.Key);
            foreach (var s in pair.Value.Similar ?? Enumerable.Empty<string>())
            {
                if (string.IsNullOrWhiteSpace(s)) continue;
                var key = s.Trim();
                if (!similarFrom.TryGetValue(key, out var list))
                {
                    list = new List<string>();
                    similarFrom[key] = list;
                }
                list.Add(pair.Key);
            }
        }

        var allKeys = new HashSet<string>(StringComparer.Ordinal);
        foreach (var k in definedSet) allKeys.Add(k);
        foreach (var k in gridFiles.Keys) allKeys.Add(k);
        foreach (var k in clickFiles.Keys) allKeys.Add(k);
        foreach (var k in similarFrom.Keys) allKeys.Add(k);

        foreach (var key in allKeys)
        {
            var isDefined = definedSet.Contains(key);
            gridFiles.TryGetValue(key, out var gf);
            clickFiles.TryGetValue(key, out var cf);
            similarFrom.TryGetValue(key, out var sf);

            var usage = new TagUsage(
                key: key,
                isDefined: isDefined,
                gridCount: gf?.Count ?? 0,
                clickCount: cf?.Count ?? 0,
                similarCount: sf?.Count ?? 0,
                gridFiles: (IReadOnlyList<string>)(gf ?? (IReadOnlyList<string>)Array.Empty<string>()),
                clickFiles: (IReadOnlyList<string>)(cf ?? (IReadOnlyList<string>)Array.Empty<string>()),
                similarFrom: (IReadOnlyList<string>)(sf ?? (IReadOnlyList<string>)Array.Empty<string>()));

            _map[key] = usage;
            if (isDefined) _definedKeys.Add(key);
            else _danglingKeys.Add(key);
            if (usage.IsUnused) _unusedKeys.Add(key);
        }

        _definedKeys.Sort(StringComparer.Ordinal);
        _danglingKeys.Sort(StringComparer.Ordinal);
        _unusedKeys.Sort(StringComparer.Ordinal);
    }
}
