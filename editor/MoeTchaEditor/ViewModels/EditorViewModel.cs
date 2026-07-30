using System.Collections.ObjectModel;
using System.ComponentModel;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using Microsoft.UI.Xaml.Media;
using Microsoft.UI.Xaml.Media.Imaging;
using MoeTchaEditor.Models;
using MoeTchaEditor.Services;

namespace MoeTchaEditor.ViewModels;

public partial class EditorViewModel : ObservableObject
{
    private const string EditorDataDirectoryName = ".moetcha-editor";
    private const string GridOriginalsDirectoryName = "grid-originals";

    [ObservableProperty]
    public partial EditorPack Pack { get; set; } = new();

    [ObservableProperty]
    public partial string Status { get; set; } = "就绪";

    [ObservableProperty]
    public partial TagUsageView? SelectedTag { get; set; }

    public string SelectedTagKey => SelectedTag?.Key ?? "";

    [ObservableProperty]
    public partial ClickImageDisplay? SelectedClickImage { get; set; }

    [ObservableProperty]
    public partial bool IsDirty { get; set; }

    [ObservableProperty]
    public partial bool IsBusy { get; set; }

    public ObservableCollection<GridImageDisplay> GridImageDisplays { get; } = [];
    public ObservableCollection<ClickImageDisplay> ClickImageDisplays { get; } = [];
    public ObservableCollection<TagUsageView> TagUsages { get; } = [];
    public ObservableCollection<TagUsageView> TagUsagesView { get; } = [];
    public ObservableCollection<TaggedImageDisplay> SelectedTagGridImages { get; } = [];
    public ObservableCollection<TaggedImageDisplay> SelectedTagClickImages { get; } = [];
    private readonly TagUsageIndex _tagIndex = new();
    public TagUsageIndex TagIndex => _tagIndex;

    [ObservableProperty]
    public partial bool ShowUnusedOnly { get; set; }

    [ObservableProperty]
    public partial bool SortByUsage { get; set; }

    [ObservableProperty]
    public partial string TagSearchText { get; set; } = "";

    private readonly HashSet<INotifyPropertyChanged> _observedObjects = [];
    private bool _suppressDirty;

    /// <summary>由页面注入，用于在新建/打开/关闭前确认放弃未保存修改。</summary>
    public Func<Task<bool>>? ConfirmDiscardAsync { get; set; }

    public EditorViewModel()
    {
        RebindPackGraph();
    }

    public string PackName
    {
        get => Pack.PackName;
        set
        {
            Pack.PackName = value;
            OnPropertyChanged();
            OnPropertyChanged(nameof(Title));
        }
    }

    public string Title
    {
        get
        {
            var dirtyMark = IsDirty ? " *" : "";
            return string.IsNullOrEmpty(Pack.PackDirectory)
                ? $"MoeTcha 素材编辑器{dirtyMark}"
                : $"MoeTcha — {Pack.PackName}  [{Pack.PackDirectory}]{dirtyMark}";
        }
    }

    public bool HasPack => !string.IsNullOrWhiteSpace(Pack.PackDirectory);
    public bool CanSave => HasPack && !IsBusy;
    public bool CanChangePack => !IsBusy;
    public List<string> TagKeys => TagUsages.Where(t => t.IsDefined).Select(t => t.Key).ToList();
    public bool HasSelectedClickImage => SelectedClickImage != null;
    public bool HasSelectedTagGridImages => SelectedTagGridImages.Count > 0;
    public bool HasSelectedTagClickImages => SelectedTagClickImages.Count > 0;
    public bool HasSelectedTagImages => HasSelectedTagGridImages || HasSelectedTagClickImages;

    public TagDefInfo? SelectedTagDef =>
        !string.IsNullOrEmpty(SelectedTagKey) && Pack.TagDefs.TryGetValue(SelectedTagKey, out var d) ? d : null;

    public bool HasSelectedTag => SelectedTag != null;

    public bool CanDefineSelectedTag =>
        SelectedTag != null && !string.IsNullOrEmpty(SelectedTagKey) && !Pack.TagDefs.ContainsKey(SelectedTagKey);

    partial void OnPackChanged(EditorPack value)
    {
        value.Normalize();
        RebindPackGraph();
        SelectedClickImage = null;
        OnPropertyChanged(nameof(PackName));
        OnPropertyChanged(nameof(Title));
        OnPropertyChanged(nameof(HasPack));
        OnPropertyChanged(nameof(CanSave));
        OnPropertyChanged(nameof(TagKeys));
        OnPropertyChanged(nameof(SelectedTagDef));
    }

    partial void OnIsDirtyChanged(bool value)
    {
        OnPropertyChanged(nameof(Title));
        OnPropertyChanged(nameof(CanSave));
    }

    partial void OnIsBusyChanged(bool value)
    {
        OnPropertyChanged(nameof(CanSave));
        OnPropertyChanged(nameof(CanChangePack));
    }

    partial void OnSelectedTagChanged(TagUsageView? value)
    {
        OnPropertyChanged(nameof(SelectedTagKey));
        OnPropertyChanged(nameof(SelectedTagDef));
        OnPropertyChanged(nameof(HasSelectedTag));
        OnPropertyChanged(nameof(CanDefineSelectedTag));
        RebuildSelectedTagImages();
    }

    partial void OnShowUnusedOnlyChanged(bool value) => RebuildTagUsagesView();
    partial void OnSortByUsageChanged(bool value) => RebuildTagUsagesView();
    partial void OnTagSearchTextChanged(string value) => RebuildTagUsagesView();

    partial void OnSelectedClickImageChanged(ClickImageDisplay? value)
        => OnPropertyChanged(nameof(HasSelectedClickImage));

    [RelayCommand]
    private async Task NewPack()
    {
        if (IsBusy) return;
        if (!await ConfirmDiscardIfNeeded()) return;

        IsBusy = true;
        try
        {
            var picker = new Windows.Storage.Pickers.FolderPicker
            {
                SuggestedStartLocation = Windows.Storage.Pickers.PickerLocationId.Desktop,
            };
            WinRT.Interop.InitializeWithWindow.Initialize(picker, App.WindowHandle);
            var folder = await picker.PickSingleFolderAsync();
            if (folder == null) return;

            if (File.Exists(Path.Combine(folder.Path, "meta.json")))
            {
                Status = "该目录已经包含 meta.json，请使用“打开”编辑现有素材包";
                return;
            }

            ReplacePack(PackSerializer.CreateNew(folder.Path));
            Status = $"已创建：{Pack.PackName}";
        }
        catch (Exception ex)
        {
            Status = $"新建失败：{ex.Message}";
        }
        finally
        {
            IsBusy = false;
        }
    }

    [RelayCommand]
    private async Task OpenPack()
    {
        if (IsBusy) return;
        if (!await ConfirmDiscardIfNeeded()) return;

        IsBusy = true;
        try
        {
            var picker = new Windows.Storage.Pickers.FileOpenPicker
            {
                SuggestedStartLocation = Windows.Storage.Pickers.PickerLocationId.Desktop,
            };
            picker.FileTypeFilter.Add(".json");
            WinRT.Interop.InitializeWithWindow.Initialize(picker, App.WindowHandle);
            var file = await picker.PickSingleFileAsync();
            if (file == null) return;
            if (!string.Equals(file.Name, "meta.json", StringComparison.OrdinalIgnoreCase))
            {
                Status = "请选择素材包目录中的 meta.json";
                return;
            }

            var loaded = PackSerializer.Load(file.Path);
            ReplacePack(loaded);
            var warnings = PackValidator.Validate(Pack);
            Status = warnings.Count == 0
                ? $"已加载：{Pack.PackName}（{Pack.GridImages.Count} Grid / {Pack.ClickImages.Count} Click）"
                : $"已加载，但有 {warnings.Count} 个问题：{warnings[0]}";
        }
        catch (Exception ex)
        {
            Status = $"打开失败：{ex.Message}";
        }
        finally
        {
            IsBusy = false;
        }
    }

    [RelayCommand]
    private void Save()
    {
        if (IsBusy)
        {
            Status = "请先完成当前操作";
            return;
        }

        if (!HasPack)
        {
            Status = "请先新建或打开一个素材包";
            return;
        }

        try
        {
            var errors = PackValidator.Validate(Pack);
            if (errors.Count > 0)
            {
                Status = errors.Count == 1
                    ? $"无法保存：{errors[0]}"
                    : $"无法保存：{errors[0]}（共 {errors.Count} 个问题）";
                return;
            }

            PackSerializer.Save(Pack);
            Pack.IsNew = false;
            IsDirty = false;
            var dangling = _tagIndex.DanglingKeys;
            Status = dangling.Count == 0
                ? $"已保存 — {DateTime.Now:HH:mm:ss}"
                : $"已保存 — {DateTime.Now:HH:mm:ss}（{dangling.Count} 个未定义标签: {string.Join(", ", dangling)}）";
        }
        catch (Exception ex)
        {
            Status = $"保存失败：{ex.Message}";
        }
    }

    [RelayCommand]
    private void AddTag()
    {
        var key = "new_tag";
        var n = 1;
        while (Pack.TagDefs.ContainsKey(key)) key = $"new_tag_{n++}";
        Pack.TagDefs[key] = new TagDefInfo { Name = key };
        RebindPackGraph();
        RefreshTags();
        SelectedTag = TagUsages.FirstOrDefault(t => t.Key == key);
        MarkDirty();
        Status = $"已添加：{key}";
    }

    [RelayCommand]
    private void DeleteTag()
    {
        if (SelectedTag == null) return;
        var key = SelectedTag.Key;
        var usage = _tagIndex.Get(key);
        if (usage != null && (usage.GridCount + usage.ClickCount + usage.SimilarCount) > 0)
        {
            Status = $"无法删除「{key}」：仍被引用 — {BuildUsageDetail(usage)}";
            return;
        }

        if (!Pack.TagDefs.Remove(key)) return;

        RebindPackGraph();
        RefreshTags();
        SelectedTag = TagUsages.FirstOrDefault(t => t.IsDefined);
        MarkDirty();
        Status = "标签已删除";
    }

    [RelayCommand]
    private void DefineSelectedTag()
    {
        if (SelectedTag == null || string.IsNullOrEmpty(SelectedTagKey)) return;
        if (Pack.TagDefs.ContainsKey(SelectedTagKey)) return;

        Pack.TagDefs[SelectedTagKey] = new TagDefInfo { Name = SelectedTagKey };
        RebindPackGraph();
        RefreshTags();
        MarkDirty();
        Status = $"已定义标签：{SelectedTagKey}";
    }

    private static string BuildUsageDetail(TagUsage u)
    {
        var parts = new List<string>();
        if (u.GridCount > 0) parts.Add($"{u.GridCount} Grid 图片");
        if (u.ClickCount > 0) parts.Add($"{u.ClickCount} Click 区域");
        if (u.SimilarCount > 0) parts.Add($"{u.SimilarCount} 相似标签");
        return parts.Count == 0 ? "无引用" : string.Join(" · ", parts);
    }

    [RelayCommand]
    private async Task ImportGridImage()
    {
        if (IsBusy) return;
        if (!HasPack)
        {
            Status = "请先新建或打开一个素材包";
            return;
        }

        IsBusy = true;
        try
        {
            var files = await PickImageFiles();
            if (files == null || files.Count == 0) return;

            Status = $"正在导入 {files.Count} 张 Grid 图片…";
            var inputs = files.Select(f => new ImageInput(f.Path, f.Name)).ToList();
            var converted = await ConvertImagesAsync(inputs);
            foreach (var image in converted)
                Pack.GridImages.Add(new GridImageInfo { File = image.FileName });

            RebindPackGraph();
            RefreshGridImages();
            MarkDirty();
            Status = $"导入了 {converted.Count} 张 Grid 图片（保留完整画面，可按需裁剪）";
        }
        catch (Exception ex)
        {
            Status = $"导入失败：{ex.Message}";
        }
        finally
        {
            IsBusy = false;
        }
    }

    [RelayCommand]
    private void DeleteGridImage(GridImageDisplay? gid)
    {
        if (gid == null) return;
        if (!Pack.GridImages.Remove(gid.Source)) return;

        DeleteImageFileIfUnreferenced(gid.Source.File);
        DeleteGridOriginalIfUnreferenced(gid.Source.File);
        RebindPackGraph();
        RefreshGridImages();
        MarkDirty();
        Status = $"已移除：{gid.FileName}";
    }

    [RelayCommand]
    private async Task ImportClickImage()
    {
        if (IsBusy) return;
        if (!HasPack)
        {
            Status = "请先新建或打开一个素材包";
            return;
        }

        IsBusy = true;
        try
        {
            var files = await PickImageFiles();
            if (files == null || files.Count == 0) return;

            Status = $"正在导入 {files.Count} 张 Click 图片…";
            var inputs = files.Select(f => new ImageInput(f.Path, f.Name)).ToList();
            var converted = await ConvertImagesAsync(inputs);
            foreach (var image in converted)
                Pack.ClickImages.Add(new ClickImageInfo { File = image.FileName });

            RebindPackGraph();
            RefreshClickImages();
            MarkDirty();
            Status = $"导入了 {converted.Count} 张 Click 图片（已转 WebP）";
        }
        catch (Exception ex)
        {
            Status = $"导入失败：{ex.Message}";
        }
        finally
        {
            IsBusy = false;
        }
    }

    [RelayCommand]
    private void DeleteClickImage(ClickImageDisplay? cid)
    {
        if (cid == null) return;
        if (!Pack.ClickImages.Remove(cid.Source)) return;

        DeleteImageFileIfUnreferenced(cid.Source.File);
        if (SelectedClickImage == cid) SelectedClickImage = null;
        RebindPackGraph();
        RefreshClickImages();
        MarkDirty();
        Status = $"已移除：{cid.FileName}";
    }

    private static async Task<IReadOnlyList<Windows.Storage.StorageFile>?> PickImageFiles()
    {
        var picker = new Windows.Storage.Pickers.FileOpenPicker
        {
            SuggestedStartLocation = Windows.Storage.Pickers.PickerLocationId.PicturesLibrary,
        };
        picker.FileTypeFilter.Add(".webp");
        picker.FileTypeFilter.Add(".png");
        picker.FileTypeFilter.Add(".jpg");
        picker.FileTypeFilter.Add(".jpeg");
        WinRT.Interop.InitializeWithWindow.Initialize(picker, App.WindowHandle);
        return await picker.PickMultipleFilesAsync();
    }

    public void RefreshAll()
    {
        Pack.Normalize();
        SelectedClickImage = null;
        if (SelectedTag == null || !Pack.TagDefs.ContainsKey(SelectedTag.Key))
            SelectedTag = TagUsages.FirstOrDefault(t => t.IsDefined);

        OnPropertyChanged(nameof(Pack));
        OnPropertyChanged(nameof(TagKeys));
        RefreshTags();
        RefreshGridImages();
        RefreshClickImages();
    }

    public void RefreshTags()
    {
        OnPropertyChanged(nameof(TagKeys));
        OnPropertyChanged(nameof(SelectedTagDef));
    }

    public void RefreshGridImages()
    {
        GridImageDisplays.Clear();
        foreach (var img in Pack.GridImages)
            if (img != null)
                GridImageDisplays.Add(new GridImageDisplay(img, Pack.PackDirectory ?? ""));
    }

    public void RefreshClickImages()
    {
        var selectedSource = SelectedClickImage?.Source;
        ClickImageDisplays.Clear();
        foreach (var img in Pack.ClickImages)
            if (img != null)
                ClickImageDisplays.Add(new ClickImageDisplay(img, Pack.PackDirectory ?? ""));

        SelectedClickImage = selectedSource == null
            ? null
            : ClickImageDisplays.FirstOrDefault(d => ReferenceEquals(d.Source, selectedSource));
    }

    public void ReplaceGridImageFile(GridImageDisplay display, string newFileName)
    {
        var oldFileName = display.Source.File;
        display.Source.File = newFileName;
        MoveGridOriginalIfNeeded(oldFileName, newFileName);
        if (!string.Equals(oldFileName, newFileName, StringComparison.OrdinalIgnoreCase))
            DeleteImageFileIfUnreferenced(oldFileName);
        RefreshGridImages();
        MarkDirty();
    }

    public string GetCropOutputFileName(GridImageDisplay display)
    {
        var outputDir = Pack.PackDirectory ?? throw new InvalidOperationException("素材包目录未设置");
        var stem = Path.GetFileNameWithoutExtension(display.Source.File);
        if (string.IsNullOrWhiteSpace(stem)) stem = "image";

        var usedStems = new HashSet<string>(StringComparer.OrdinalIgnoreCase);
        foreach (var image in Pack.GridImages)
        {
            if (image != null && !ReferenceEquals(image, display.Source))
                usedStems.Add(Path.GetFileNameWithoutExtension(image.File));
        }
        foreach (var image in Pack.ClickImages)
        {
            if (image != null)
                usedStems.Add(Path.GetFileNameWithoutExtension(image.File));
        }

        var candidateStem = stem;
        var candidate = $"{candidateStem}.webp";
        var currentFile = Path.GetFileName(display.Source.File);
        var suffix = 1;
        while (usedStems.Contains(candidateStem)
               || File.Exists(Path.Combine(outputDir, candidate))
                  && !string.Equals(candidate, currentFile, StringComparison.OrdinalIgnoreCase))
        {
            candidateStem = $"{stem}_{suffix++}";
            candidate = $"{candidateStem}.webp";
        }
        return candidate;
    }

    public string EnsureGridCropSource(GridImageDisplay display)
    {
        var packDirectory = Pack.PackDirectory ?? throw new InvalidOperationException("素材包目录未设置");
        if (!File.Exists(display.FullPath))
            throw new FileNotFoundException("Grid 原图不存在", display.FullPath);

        var originalPath = GetGridOriginalPath(packDirectory, display.Source.File);
        if (File.Exists(originalPath)) return originalPath;

        var originalDirectory = Path.GetDirectoryName(originalPath)
            ?? throw new InvalidOperationException("原图缓存目录无效");
        Directory.CreateDirectory(originalDirectory);
        TryHideEditorDataDirectory(packDirectory);

        if (string.Equals(Path.GetExtension(display.FullPath), ".webp", StringComparison.OrdinalIgnoreCase))
            CopyFileAtomically(display.FullPath, originalPath);
        else
            ImageProcessor.ToWebp(display.FullPath, originalDirectory, Path.GetFileName(originalPath));

        return originalPath;
    }

    public void MarkDirty()
    {
        if (!_suppressDirty && HasPack)
            IsDirty = true;
    }

    public void NotifyPackStructureChanged()
    {
        RebindPackGraph();
        MarkDirty();
    }

    private async Task<bool> ConfirmDiscardIfNeeded()
    {
        if (!IsDirty) return true;
        if (ConfirmDiscardAsync == null) return false;

        try
        {
            return await ConfirmDiscardAsync();
        }
        catch (Exception ex)
        {
            Status = $"无法确认未保存修改：{ex.Message}";
            return false;
        }
    }

    private void ReplacePack(EditorPack pack)
    {
        _suppressDirty = true;
        try
        {
            pack.Normalize();
            Pack = pack;
            IsDirty = pack.IsNew;
            RefreshAll();
        }
        finally
        {
            _suppressDirty = false;
        }
    }

    private void RebindPackGraph()
    {
        foreach (var observed in _observedObjects)
            observed.PropertyChanged -= ObservedObjectPropertyChanged;
        _observedObjects.Clear();

        Subscribe(Pack);
        Subscribe(Pack.Grid);
        Subscribe(Pack.Click);
        foreach (var def in Pack.TagDefs.Values)
            if (def != null)
                Subscribe(def);
        foreach (var image in Pack.GridImages)
            if (image != null)
                Subscribe(image);
        foreach (var image in Pack.ClickImages)
        {
            if (image == null) continue;
            Subscribe(image);
            foreach (var region in image.Regions)
                if (region != null)
                    Subscribe(region);
        }

        RebuildTagIndex();
    }

    private void Subscribe(INotifyPropertyChanged? observed)
    {
        if (observed == null || !_observedObjects.Add(observed)) return;
        observed.PropertyChanged += ObservedObjectPropertyChanged;
    }

    private void ObservedObjectPropertyChanged(object? sender, PropertyChangedEventArgs e)
    {
        if (_suppressDirty) return;

        if (ReferenceEquals(sender, Pack))
        {
            if (e.PropertyName is nameof(EditorPack.TagDefs) or nameof(EditorPack.Grid)
                or nameof(EditorPack.Click) or nameof(EditorPack.GridImages)
                or nameof(EditorPack.ClickImages))
                RebindPackGraph();

            if (e.PropertyName == nameof(EditorPack.PackName))
                OnPropertyChanged(nameof(PackName));
            if (e.PropertyName is nameof(EditorPack.PackName) or nameof(EditorPack.PackDirectory))
            {
                OnPropertyChanged(nameof(Title));
                OnPropertyChanged(nameof(HasPack));
                OnPropertyChanged(nameof(CanSave));
            }
        }
        else if (sender is GridImageInfo && e.PropertyName == nameof(GridImageInfo.Tags))
        {
            RebuildTagIndex();
        }
        else if (sender is RegionInfo && e.PropertyName == nameof(RegionInfo.Tag))
        {
            RebuildTagIndex();
        }
        else if (sender is TagDefInfo && e.PropertyName == nameof(TagDefInfo.Similar))
        {
            RebuildTagIndex();
        }

        MarkDirty();
    }

    private void RebuildTagIndex()
    {
        var selectedKey = SelectedTag?.Key;
        _tagIndex.Rebuild(Pack);

        TagUsages.Clear();
        foreach (var u in _tagIndex.All)
            TagUsages.Add(new TagUsageView(u.Key, u.IsDefined, u.GridCount, u.ClickCount, u.SimilarCount, u.IsUnused));

        if (selectedKey != null)
        {
            var match = TagUsages.FirstOrDefault(t => string.Equals(t.Key, selectedKey, StringComparison.Ordinal));
            if (match != null && !ReferenceEquals(match, SelectedTag))
                SelectedTag = match;
        }

        OnPropertyChanged(nameof(TagKeys));
        OnPropertyChanged(nameof(SelectedTagDef));
        OnPropertyChanged(nameof(HasSelectedTag));
        OnPropertyChanged(nameof(CanDefineSelectedTag));
        RebuildTagUsagesView();
        RebuildSelectedTagImages();
    }

    /// <summary>根据当前选中的标签，重建「引用此标签的图片」缩略图列表。</summary>
    private void RebuildSelectedTagImages()
    {
        SelectedTagGridImages.Clear();
        SelectedTagClickImages.Clear();

        var key = SelectedTagKey;
        if (!string.IsNullOrEmpty(key))
        {
            var usage = _tagIndex.Get(key);
            if (usage != null)
            {
                var dir = Pack.PackDirectory ?? "";
                foreach (var file in usage.GridFiles)
                {
                    var source = Pack.GridImages.FirstOrDefault(g =>
                        g != null && string.Equals(g.File, file, StringComparison.Ordinal));
                    SelectedTagGridImages.Add(new TaggedImageDisplay(file, dir, TaggedImageKind.Grid, source));
                }
                foreach (var file in usage.ClickFiles)
                {
                    var source = Pack.ClickImages.FirstOrDefault(c =>
                        c != null && string.Equals(c.File, file, StringComparison.Ordinal));
                    SelectedTagClickImages.Add(new TaggedImageDisplay(file, dir, TaggedImageKind.Click, source));
                }
            }
        }

        OnPropertyChanged(nameof(HasSelectedTagGridImages));
        OnPropertyChanged(nameof(HasSelectedTagClickImages));
        OnPropertyChanged(nameof(HasSelectedTagImages));
    }

    private void RebuildTagUsagesView()
    {
        var selectedKey = SelectedTag?.Key;
        IEnumerable<TagUsageView> src = TagUsages;
        if (ShowUnusedOnly) src = src.Where(t => t.IsUnused);

        var search = TagSearchText?.Trim() ?? "";
        if (!string.IsNullOrEmpty(search))
            src = src.Where(t => t.Key.Contains(search, StringComparison.OrdinalIgnoreCase));

        if (SortByUsage)
            src = src.OrderByDescending(t => t.GridCount + t.ClickCount + t.SimilarCount)
                     .ThenBy(t => t.Key, StringComparer.Ordinal);
        else
            src = src.OrderBy(t => t.Key, StringComparer.Ordinal);

        TagUsagesView.Clear();
        foreach (var t in src) TagUsagesView.Add(t);

        if (selectedKey != null)
        {
            var match = TagUsagesView.FirstOrDefault(t => string.Equals(t.Key, selectedKey, StringComparison.Ordinal));
            if (match != null && !ReferenceEquals(match, SelectedTag))
                SelectedTag = match;
        }
    }

    private async Task<List<ConvertedImage>> ConvertImagesAsync(IReadOnlyList<ImageInput> inputs)
    {
        var outputDir = Pack.PackDirectory ?? throw new InvalidOperationException("素材包目录未设置");
        Directory.CreateDirectory(outputDir);

        var reservedStems = new HashSet<string>(
            Pack.GridImages.Where(image => image != null).Select(image => Path.GetFileNameWithoutExtension(image.File))
                .Concat(Pack.ClickImages.Where(image => image != null).Select(image => Path.GetFileNameWithoutExtension(image.File))),
            StringComparer.OrdinalIgnoreCase);

        var jobs = new List<ConversionJob>(inputs.Count);
        foreach (var input in inputs)
        {
            var outputName = ReserveUniqueFileName(input.Name, outputDir, reservedStems);
            jobs.Add(new ConversionJob(input.Path, outputName));
        }

        var created = new List<string>(jobs.Count);
        try
        {
            var converted = await Task.Run(() =>
            {
                var result = new List<ConvertedImage>(jobs.Count);
                foreach (var job in jobs)
                {
                    var expectedPath = Path.Combine(outputDir, job.OutputName);
                    created.Add(expectedPath);
                    var outputName = ImageProcessor.ToWebp(job.SourcePath, outputDir, job.OutputName);
                    result.Add(new ConvertedImage(outputName));
                }

                return result;
            });
            return converted;
        }
        catch
        {
            foreach (var path in created)
            {
                try
                {
                    if (File.Exists(path)) File.Delete(path);
                }
                catch
                {
                    // 保留原始导入异常；清理失败不会覆盖它。
                }
            }
            throw;
        }
    }

    private static string ReserveUniqueFileName(
        string sourceName,
        string outputDir,
        HashSet<string> reservedStems)
    {
        var stem = Path.GetFileNameWithoutExtension(sourceName);
        if (string.IsNullOrWhiteSpace(stem)) stem = "image";

        var candidateStem = stem;
        var candidate = $"{candidateStem}.webp";
        var suffix = 1;
        while (reservedStems.Contains(candidateStem) || File.Exists(Path.Combine(outputDir, candidate)))
        {
            candidateStem = $"{stem}_{suffix++}";
            candidate = $"{candidateStem}.webp";
        }

        reservedStems.Add(candidateStem);
        return candidate;
    }

    private void DeleteImageFileIfUnreferenced(string fileName)
    {
        var directory = Pack.PackDirectory;
        if (string.IsNullOrWhiteSpace(directory) || string.IsNullOrWhiteSpace(fileName)) return;

        var baseName = Path.GetFileName(fileName);
        if (!string.Equals(baseName, fileName, StringComparison.Ordinal)) return;

        var stillReferenced = Pack.GridImages.Any(image => image != null && string.Equals(
                                  Path.GetFileName(image.File), baseName, StringComparison.OrdinalIgnoreCase))
                              || Pack.ClickImages.Any(image => image != null && string.Equals(
                                  Path.GetFileName(image.File), baseName, StringComparison.OrdinalIgnoreCase));
        if (stillReferenced) return;

        try
        {
            var path = Path.Combine(directory, baseName);
            if (File.Exists(path)) File.Delete(path);
        }
        catch
        {
            // 元数据删除已经完成，文件清理失败不应阻止编辑器继续工作。
        }
    }

    private void DeleteGridOriginalIfUnreferenced(string fileName)
    {
        var directory = Pack.PackDirectory;
        if (string.IsNullOrWhiteSpace(directory) || string.IsNullOrWhiteSpace(fileName)) return;

        var stem = Path.GetFileNameWithoutExtension(fileName);
        if (string.IsNullOrWhiteSpace(stem)) return;
        if (Pack.GridImages.Any(image => image != null && string.Equals(
                Path.GetFileNameWithoutExtension(image.File), stem, StringComparison.OrdinalIgnoreCase))) return;

        try
        {
            var path = GetGridOriginalPath(directory, fileName);
            if (File.Exists(path)) File.Delete(path);
        }
        catch
        {
            // 删除素材已经完成，编辑器原图缓存清理失败不应阻止操作。
        }
    }

    private void MoveGridOriginalIfNeeded(string oldFileName, string newFileName)
    {
        var directory = Pack.PackDirectory;
        if (string.IsNullOrWhiteSpace(directory)) return;

        var oldStem = Path.GetFileNameWithoutExtension(oldFileName);
        var newStem = Path.GetFileNameWithoutExtension(newFileName);
        if (string.IsNullOrWhiteSpace(oldStem) || string.IsNullOrWhiteSpace(newStem)
            || string.Equals(oldStem, newStem, StringComparison.OrdinalIgnoreCase)) return;

        try
        {
            var oldPath = GetGridOriginalPath(directory, oldFileName);
            var newPath = GetGridOriginalPath(directory, newFileName);
            if (!File.Exists(oldPath) || File.Exists(newPath)) return;
            Directory.CreateDirectory(Path.GetDirectoryName(newPath)!);
            File.Move(oldPath, newPath);
        }
        catch
        {
            // 主图片已经成功裁剪；缓存改名失败时下次会从当前图片重新建立。
        }
    }

    private static string GetGridOriginalPath(string packDirectory, string fileName)
    {
        var stem = Path.GetFileNameWithoutExtension(Path.GetFileName(fileName));
        if (string.IsNullOrWhiteSpace(stem))
            throw new InvalidOperationException("Grid 图片文件名无效");
        return Path.Combine(packDirectory, EditorDataDirectoryName, GridOriginalsDirectoryName, $"{stem}.webp");
    }

    private static void CopyFileAtomically(string sourcePath, string targetPath)
    {
        var directory = Path.GetDirectoryName(targetPath)
            ?? throw new InvalidOperationException("原图缓存目录无效");
        var temporary = Path.Combine(directory, $".{Path.GetFileName(targetPath)}.{Guid.NewGuid():N}.tmp");
        try
        {
            File.Copy(sourcePath, temporary, true);
            File.Move(temporary, targetPath, true);
        }
        finally
        {
            try
            {
                if (File.Exists(temporary)) File.Delete(temporary);
            }
            catch
            {
                // 保留原始复制异常。
            }
        }
    }

    private static void TryHideEditorDataDirectory(string packDirectory)
    {
        try
        {
            var path = Path.Combine(packDirectory, EditorDataDirectoryName);
            var info = new DirectoryInfo(path);
            info.Attributes |= FileAttributes.Hidden;
        }
        catch
        {
            // 隐藏属性仅影响显示，不影响裁剪功能。
        }
    }

    private sealed record ImageInput(string Path, string Name);
    private sealed record ConversionJob(string SourcePath, string OutputName);
    private sealed record ConvertedImage(string FileName);
}

public class GridImageDisplay
{
    public GridImageInfo Source { get; }
    public string FileName => Source.File;
    public string FullPath { get; }
    public ImageSource? Thumbnail { get; }

    public GridImageDisplay(GridImageInfo src, string packDir)
    {
        Source = src;
        FullPath = SafePath(packDir, src.File);
        if (File.Exists(FullPath))
        {
            try
            {
                Thumbnail = new BitmapImage
                {
                    CreateOptions = BitmapCreateOptions.IgnoreImageCache,
                    UriSource = new Uri(Path.GetFullPath(FullPath)),
                };
            }
            catch { }
        }
    }

    private static string SafePath(string packDir, string file)
        => Path.Combine(packDir, Path.GetFileName(file));
}

public class ClickImageDisplay
{
    public ClickImageInfo Source { get; }
    public string FileName => Source.File;
    public string FullPath { get; }
    public ImageSource? Thumbnail { get; }

    public ClickImageDisplay(ClickImageInfo src, string packDir)
    {
        Source = src;
        FullPath = SafePath(packDir, src.File);
        if (File.Exists(FullPath))
        {
            try { Thumbnail = new BitmapImage(new Uri(Path.GetFullPath(FullPath))); }
            catch { }
        }
    }

    private static string SafePath(string packDir, string file)
        => Path.Combine(packDir, Path.GetFileName(file));
}

public sealed class TagUsageView
{
    public string Key { get; }
    public bool IsDefined { get; }
    public int GridCount { get; }
    public int ClickCount { get; }
    public int SimilarCount { get; }
    public bool IsUnused { get; }

    public string Summary
    {
        get
        {
            var parts = new List<string>(3);
            if (GridCount > 0) parts.Add($"Grid {GridCount}");
            if (ClickCount > 0) parts.Add($"Click {ClickCount}");
            if (SimilarCount > 0) parts.Add($"Similar {SimilarCount}");
            return parts.Count == 0 ? "无引用" : string.Join(" · ", parts);
        }
    }

    public TagUsageView(string key, bool isDefined, int gridCount, int clickCount, int similarCount, bool isUnused)
    {
        Key = key;
        IsDefined = isDefined;
        GridCount = gridCount;
        ClickCount = clickCount;
        SimilarCount = similarCount;
        IsUnused = isUnused;
    }
}

public enum TaggedImageKind
{
    Grid,
    Click,
}

/// <summary>「按标签查看图片」中单张图片的展示对象，加载缩略图并保留源对象以便后续跳转。</summary>
public sealed class TaggedImageDisplay
{
    public string FileName { get; }
    public string FullPath { get; }
    public ImageSource? Thumbnail { get; }
    public TaggedImageKind Kind { get; }
    public object? Source { get; }

    public TaggedImageDisplay(string fileName, string packDir, TaggedImageKind kind, object? source)
    {
        FileName = fileName;
        Kind = kind;
        Source = source;
        FullPath = Path.Combine(packDir, Path.GetFileName(fileName));
        if (File.Exists(FullPath))
        {
            try
            {
                Thumbnail = new BitmapImage
                {
                    CreateOptions = BitmapCreateOptions.IgnoreImageCache,
                    UriSource = new Uri(Path.GetFullPath(FullPath)),
                };
            }
            catch { }
        }
    }
}
