using System.Text.Json.Serialization;
using System.Collections.ObjectModel;
using CommunityToolkit.Mvvm.ComponentModel;

namespace MoeTchaEditor.Models;

public class TagDefInfo : ObservableObject
{
    private string _name = "";
    private List<string> _similar = [];

    [JsonPropertyName("name")]
    public string Name
    {
        get => _name;
        set => SetProperty(ref _name, value ?? "");
    }

    [JsonPropertyName("similar")]
    public List<string> Similar
    {
        get => _similar;
        set => SetProperty(ref _similar, value ?? []);
    }

    [JsonIgnore]
    public string SimilarStr
    {
        get => string.Join("\n", Similar);
        set
        {
            var values = (value ?? "")
                .Split('\n')
                .Select(line => line.Trim())
                .Where(line => !string.IsNullOrEmpty(line))
                .Distinct(StringComparer.Ordinal)
                .ToList();
            Similar = values;
            OnPropertyChanged();
        }
    }
}

public class GridConfigInfo : ObservableObject
{
    private int _size = 9;
    private int _correctMin = 2;
    private int _correctMax = 4;
    private string _question = "请选出所有「{tag}」";

    [JsonPropertyName("size")]
    public int Size { get => _size; set => SetProperty(ref _size, value); }

    [JsonPropertyName("correct_min")]
    public int CorrectMin { get => _correctMin; set => SetProperty(ref _correctMin, value); }

    [JsonPropertyName("correct_max")]
    public int CorrectMax { get => _correctMax; set => SetProperty(ref _correctMax, value); }

    [JsonPropertyName("question")]
    public string Question
    {
        get => _question;
        set => SetProperty(ref _question, value ?? "");
    }
}

public class ClickConfigInfo : ObservableObject
{
    private string _question = "请点击图中所有「{tag}」";

    [JsonPropertyName("question")]
    public string Question
    {
        get => _question;
        set => SetProperty(ref _question, value ?? "");
    }
}

public class GridImageInfo : ObservableObject
{
    private string _file = "";
    private List<string> _tags = [];

    [JsonPropertyName("file")]
    public string File
    {
        get => _file;
        set => SetProperty(ref _file, value ?? "");
    }

    [JsonPropertyName("tags")]
    public List<string> Tags
    {
        get => _tags;
        set => SetProperty(ref _tags, value ?? []);
    }

    [JsonIgnore]
    public string TagsStr
    {
        get => string.Join(", ", Tags);
        set
        {
            var values = (value ?? "")
                .Split(',')
                .Select(part => part.Trim())
                .Where(part => !string.IsNullOrEmpty(part))
                .Distinct(StringComparer.Ordinal)
                .ToList();
            Tags = values;
            OnPropertyChanged();
        }
    }
}

public class RegionInfo : ObservableObject
{
    private string _tag = "";
    private int _x;
    private int _y;
    private int _width;
    private int _height;

    [JsonPropertyName("tag")]
    public string Tag { get => _tag; set => SetProperty(ref _tag, value?.Trim() ?? ""); }

    [JsonPropertyName("x")]
    public int X { get => _x; set => SetProperty(ref _x, value); }

    [JsonPropertyName("y")]
    public int Y { get => _y; set => SetProperty(ref _y, value); }

    [JsonPropertyName("width")]
    public int Width { get => _width; set => SetProperty(ref _width, value); }

    [JsonPropertyName("height")]
    public int Height { get => _height; set => SetProperty(ref _height, value); }
}

public class ClickImageInfo : ObservableObject
{
    private string _file = "";
    private ObservableCollection<RegionInfo> _regions = [];

    [JsonPropertyName("file")]
    public string File
    {
        get => _file;
        set => SetProperty(ref _file, value ?? "");
    }

    [JsonPropertyName("regions")]
    public ObservableCollection<RegionInfo> Regions
    {
        get => _regions;
        set => SetProperty(ref _regions, value ?? []);
    }
}

public class EditorPack : ObservableObject
{
    private string _packName = "";
    private string _author = "";
    private string _version = "";
    private string _description = "";
    private Dictionary<string, TagDefInfo> _tagDefs = [];
    private GridConfigInfo _grid = new();
    private ClickConfigInfo _click = new();
    private List<GridImageInfo> _gridImages = [];
    private List<ClickImageInfo> _clickImages = [];
    private Dictionary<string, object?>? _extra;
    private string? _packDirectory;
    private bool _isNew = true;

    [JsonPropertyName("pack_name")]
    public string PackName { get => _packName; set => SetProperty(ref _packName, value ?? ""); }

    [JsonPropertyName("author")]
    public string Author { get => _author; set => SetProperty(ref _author, value ?? ""); }

    [JsonPropertyName("version")]
    public string Version { get => _version; set => SetProperty(ref _version, value ?? ""); }

    [JsonPropertyName("description")]
    public string Description { get => _description; set => SetProperty(ref _description, value ?? ""); }

    [JsonPropertyName("tag_defs")]
    public Dictionary<string, TagDefInfo> TagDefs
    {
        get => _tagDefs;
        set => SetProperty(ref _tagDefs, value ?? []);
    }

    [JsonPropertyName("grid")]
    public GridConfigInfo Grid
    {
        get => _grid;
        set => SetProperty(ref _grid, value ?? new());
    }

    [JsonPropertyName("click")]
    public ClickConfigInfo Click
    {
        get => _click;
        set => SetProperty(ref _click, value ?? new());
    }

    [JsonPropertyName("grid_images")]
    public List<GridImageInfo> GridImages
    {
        get => _gridImages;
        set => SetProperty(ref _gridImages, value ?? []);
    }

    [JsonPropertyName("click_images")]
    public List<ClickImageInfo> ClickImages
    {
        get => _clickImages;
        set => SetProperty(ref _clickImages, value ?? []);
    }

    [JsonPropertyName("extra")]
    public Dictionary<string, object?>? Extra
    {
        get => _extra;
        set => SetProperty(ref _extra, value);
    }

    [JsonIgnore]
    public string? PackDirectory
    {
        get => _packDirectory;
        set => SetProperty(ref _packDirectory, value);
    }

    [JsonIgnore]
    public bool IsNew
    {
        get => _isNew;
        set => SetProperty(ref _isNew, value);
    }

    /// <summary>把来自外部 JSON 的 null 集合/配置恢复为可编辑状态。</summary>
    public void Normalize()
    {
        TagDefs ??= [];
        Grid ??= new();
        Click ??= new();
        GridImages ??= [];
        ClickImages ??= [];

        foreach (var def in TagDefs.Values)
        {
            if (def != null) def.Similar ??= [];
        }

        foreach (var image in GridImages)
        {
            if (image != null) image.Tags ??= [];
        }

        foreach (var image in ClickImages)
        {
            if (image == null) continue;
            image.Regions ??= [];
            foreach (var region in image.Regions)
            {
                if (region != null) region.Tag ??= "";
            }
        }
    }
}
