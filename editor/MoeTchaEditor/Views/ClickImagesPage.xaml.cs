using System.Collections.ObjectModel;
using System.ComponentModel;
using System.Runtime.CompilerServices;
using Microsoft.UI;
using Microsoft.UI.Dispatching;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Controls.Primitives;
using Microsoft.UI.Xaml.Input;
using Microsoft.UI.Xaml.Media;
using Microsoft.UI.Xaml.Media.Imaging;
using Microsoft.UI.Xaml.Shapes;
using Windows.Foundation;
using MoeTchaEditor.Models;
using MoeTchaEditor.ViewModels;

namespace MoeTchaEditor.Views;

public sealed partial class ClickImagesPage : Page, INotifyPropertyChanged
{
    private EditorViewModel? _subscribedVm;
    private PropertyChangedEventHandler? _vmPropertyChanged;
    private readonly List<RegionInfo> _observedRegions = [];
    private ObservableCollection<RegionInfo>? _observedRegionsCollection;

    // 画布几何
    private double _scale;
    private double _ox, _oy;
    private int _imgW, _imgH;

    // 绘制新区域
    private Point _drawStart;

    // 选中 / 拖拽
    private enum DragMode { None, Draw, Move, ResizeNW, ResizeNE, ResizeSW, ResizeSE }
    private DragMode _mode = DragMode.None;
    private RegionInfo? _selected;
    private RegionInfo? _dragRegion;
    private Rect _dragStartRect;   // 拖拽起始时的区域（图像坐标）
    private Point _dragStartScreen; // 拖拽起始指针（画布坐标）

    // 标签配色（按首次出现顺序分配）
    private readonly Dictionary<string, SolidColorBrush> _tagColors = [];
    private static readonly Windows.UI.Color[] Palette =
    [
        Windows.UI.Color.FromArgb(0xFF, 0x00, 0x78, 0xD4), // 蓝
        Windows.UI.Color.FromArgb(0xFF, 0xE3, 0x00, 0x8C), // 品红
        Windows.UI.Color.FromArgb(0xFF, 0x10, 0x7C, 0x10), // 绿
        Windows.UI.Color.FromArgb(0xFF, 0xC1, 0x9C, 0x00), // 金
        Windows.UI.Color.FromArgb(0xFF, 0xC2, 0x39, 0xB3), // 紫
        Windows.UI.Color.FromArgb(0xFF, 0x00, 0x85, 0x75), // 青绿
        Windows.UI.Color.FromArgb(0xFF, 0xCA, 0x50, 0x10), // 橙
        Windows.UI.Color.FromArgb(0xFF, 0x6B, 0x69, 0xD6), // 靛
    ];

    /// <summary>当前绘制目标标签：新建区域自动归入此标签。null 表示未指定（新建区域为空标签）。</summary>
    private string? _activeTag;

    /// <summary>右侧分组列表（稳定引用，按需 Clear/Add）。</summary>
    public ObservableCollection<RegionGroup> RegionGroups { get; } = [];

    private int _regionCount;
    public int RegionCount
    {
        get => _regionCount;
        set { if (_regionCount != value) { _regionCount = value; Raise(); } }
    }

    public event PropertyChangedEventHandler? PropertyChanged;

    // x:Bind 可能在加载/卸载边界被调用，必须无副作用。
    private EditorViewModel? VM => DataContext as EditorViewModel;

    public ClickImagesPage()
    {
        InitializeComponent();
        DataContextChanged += OnDataContextChanged;
        Loaded += OnLoaded;
        Unloaded += OnUnloaded;
    }

    private void Raise([CallerMemberName] string? name = null)
        => PropertyChanged?.Invoke(this, new PropertyChangedEventArgs(name));

    private void OnDataContextChanged(FrameworkElement sender, DataContextChangedEventArgs args)
    {
        if (IsLoaded) AttachViewModel();
    }

    private void OnLoaded(object sender, RoutedEventArgs e)
        => AttachViewModel();

    private void OnUnloaded(object sender, RoutedEventArgs e)
    {
        _mode = DragMode.None;
        RegionCanvas.ReleasePointerCaptures();
        DetachViewModel();
        ClearGeometry();
    }

    private void AttachViewModel()
    {
        var vm = VM;
        if (ReferenceEquals(_subscribedVm, vm)) return;

        DetachViewModel();
        if (vm == null) return;

        _subscribedVm = vm;
        _vmPropertyChanged = (_, args) =>
        {
            if (args.PropertyName is nameof(EditorViewModel.SelectedClickImage))
            {
                _selected = null;
                AttachRegionHandlers();
                ClearGeometry();
                Redraw();
                // Image.ImageOpened 不会为「已解码/缓存」的 BitmapImage 再次触发：左侧图片列表会
                // 预解码每个缩略图（与 Preview 共用同一 Thumbnail 实例），切换选中后 Preview 复用
                // 同一已加载源，ImageOpened 不再 fire，致使 _scale 停在 0 —— 区域不绘制、也无法在
                // 画布上新建区域。这里在绑定传播完新 Source 后主动重算一次几何兜底。
                DispatcherQueue.TryEnqueue(DispatcherQueuePriority.Normal, UpdateImageGeometry);
            }
            else if (args.PropertyName is nameof(EditorViewModel.ClickImageDisplays))
            {
                AttachRegionHandlers();
                Redraw();
            }
        };
        _subscribedVm.PropertyChanged += _vmPropertyChanged;
        _selected = null;
        AttachRegionHandlers();
    }

    private void DetachViewModel()
    {
        if (_subscribedVm != null && _vmPropertyChanged != null)
            _subscribedVm.PropertyChanged -= _vmPropertyChanged;
        _vmPropertyChanged = null;
        DetachRegionHandlers();
        _subscribedVm = null;
    }

    private void AttachRegionHandlers()
    {
        DetachRegionHandlers();
        var src = _subscribedVm?.SelectedClickImage;
        if (src == null) { RebuildGroups(); return; }

        var regions = src.Source.Regions;
        _observedRegionsCollection = regions;
        regions.CollectionChanged += RegionsCollectionChanged;

        foreach (var region in regions)
        {
            if (region == null) continue;
            region.PropertyChanged += RegionPropertyChanged;
            _observedRegions.Add(region);
        }
        RebuildGroups();
    }

    private void DetachRegionHandlers()
    {
        if (_observedRegionsCollection != null)
            _observedRegionsCollection.CollectionChanged -= RegionsCollectionChanged;
        _observedRegionsCollection = null;

        foreach (var region in _observedRegions)
            region.PropertyChanged -= RegionPropertyChanged;
        _observedRegions.Clear();
    }

    private void RegionsCollectionChanged(object? sender, System.Collections.Specialized.NotifyCollectionChangedEventArgs e)
    {
        // 重新挂接每个区域的 PropertyChanged
        foreach (var region in _observedRegions)
            region.PropertyChanged -= RegionPropertyChanged;
        _observedRegions.Clear();

        var regions = _observedRegionsCollection;
        if (regions != null)
        {
            foreach (var region in regions)
            {
                if (region == null) continue;
                region.PropertyChanged += RegionPropertyChanged;
                _observedRegions.Add(region);
            }
        }
        RebuildGroups();
        Redraw();
    }

    private void RegionPropertyChanged(object? sender, PropertyChangedEventArgs e)
    {
        if (e.PropertyName == nameof(RegionInfo.Tag))
        {
            // TagEditor 的标签提交是原子的（回车/点选，无中途输入态），可安全重建分组让区域在分组间迁移；
            // 延迟到提交流程结束再重建，避免在 TagEditor 内部替换其所在行导致焦点/弹层异常。
            Redraw();
            DispatcherQueue.TryEnqueue(DispatcherQueuePriority.Normal, RebuildGroups);
        }
        else if (e.PropertyName is nameof(RegionInfo.X) or nameof(RegionInfo.Y)
            or nameof(RegionInfo.Width) or nameof(RegionInfo.Height))
        {
            Redraw();
        }
    }

    // ─────────────────── 图像几何 ───────────────────

    private void OnImageOpened(object sender, RoutedEventArgs e)
        => UpdateImageGeometry();

    private void OnImageFailed(object sender, ExceptionRoutedEventArgs e)
    {
        ClearGeometry();
        Redraw();
    }

    private void OnCanvasSizeChanged(object sender, SizeChangedEventArgs e)
        => UpdateImageGeometry();

    private void UpdateImageGeometry()
    {
        if (Preview.Source is not BitmapSource bitmap || bitmap.PixelWidth <= 0 || bitmap.PixelHeight <= 0
            || RegionCanvas.ActualWidth <= 0 || RegionCanvas.ActualHeight <= 0)
        {
            ClearGeometry();
            Redraw();
            return;
        }

        _imgW = bitmap.PixelWidth;
        _imgH = bitmap.PixelHeight;
        _scale = Math.Min(RegionCanvas.ActualWidth / _imgW, RegionCanvas.ActualHeight / _imgH);
        _ox = (RegionCanvas.ActualWidth - _imgW * _scale) / 2;
        _oy = (RegionCanvas.ActualHeight - _imgH * _scale) / 2;
        Redraw();
    }

    private void ClearGeometry()
    {
        _imgW = _imgH = 0;
        _scale = 0;
        _ox = _oy = 0;
    }

    private Point ToScreen(double ix, double iy)
        => new(_ox + ix * _scale, _oy + iy * _scale);

    private double ToImageX(double sx) => (sx - _ox) / _scale;
    private double ToImageY(double sy) => (sy - _oy) / _scale;

    private Rect RegionScreenRect(RegionInfo r)
    {
        var p = ToScreen(r.X, r.Y);
        return new Rect(p.X, p.Y, r.Width * _scale, r.Height * _scale);
    }

    // ─────────────────── 指针交互 ───────────────────

    private void CanvasPressed(object sender, PointerRoutedEventArgs e)
    {
        if (_subscribedVm?.SelectedClickImage == null || _imgW <= 0 || _imgH <= 0 || _scale <= 0) return;

        var p = e.GetCurrentPoint(RegionCanvas).Position;
        e.Handled = true;

        // 1) 优先：选中区域的调整句柄
        if (_selected != null && HitHandle(_selected, p, out var handle))
        {
            _mode = handle;
            _dragRegion = _selected;
            _dragStartRect = new Rect(_selected.X, _selected.Y, _selected.Width, _selected.Height);
            _dragStartScreen = p;
            RegionCanvas.CapturePointer(e.Pointer);
            return;
        }

        // 2) 命中区域主体：选中并移动（顶层优先 = 列表末尾）
        var regions = _subscribedVm.SelectedClickImage.Source.Regions;
        for (int i = regions.Count - 1; i >= 0; i--)
        {
            var r = regions[i];
            if (r == null || r.Width <= 0 || r.Height <= 0) continue;
            if (RegionScreenRect(r).Contains(p))
            {
                Select(r);
                _mode = DragMode.Move;
                _dragRegion = r;
                _dragStartRect = new Rect(r.X, r.Y, r.Width, r.Height);
                _dragStartScreen = p;
                RegionCanvas.CapturePointer(e.Pointer);
                return;
            }
        }

        // 3) 空白处：绘制新区域
        _mode = DragMode.Draw;
        _drawStart = p;
        Select(null);
        RegionCanvas.CapturePointer(e.Pointer);
    }

    private void CanvasMoved(object sender, PointerRoutedEventArgs e)
    {
        if (_mode == DragMode.None) return;
        var p = e.GetCurrentPoint(RegionCanvas).Position;
        e.Handled = true;

        switch (_mode)
        {
            case DragMode.Draw:
                Redraw(); // 清掉旧预览
                DrawPreview(_drawStart, p);
                break;
            case DragMode.Move when _dragRegion != null:
                {
                    var dx = ToImageX(p.X) - ToImageX(_dragStartScreen.X);
                    var dy = ToImageY(p.Y) - ToImageY(_dragStartScreen.Y);
                    _dragRegion.X = Clamp((int)Math.Round(_dragStartRect.X + dx), 0, Math.Max(0, _imgW - _dragRegion.Width));
                    _dragRegion.Y = Clamp((int)Math.Round(_dragStartRect.Y + dy), 0, Math.Max(0, _imgH - _dragRegion.Height));
                    Redraw();
                    break;
                }
            case DragMode.ResizeNW:
            case DragMode.ResizeNE:
            case DragMode.ResizeSW:
            case DragMode.ResizeSE when _dragRegion != null:
                ApplyResize(_mode, p);
                Redraw();
                break;
        }
    }

    private void CanvasReleased(object sender, PointerRoutedEventArgs e)
    {
        if (_mode == DragMode.None) { return; }
        var p = e.GetCurrentPoint(RegionCanvas).Position;
        e.Handled = true;

        // 必须先读出 mode 并清零，再释放捕获：
        // ReleasePointerCaptures() 会同步触发 PointerCaptureLost -> CanvasPointerCanceled，
        // 若先释放再读，mode 已被重置为 None，绘制将永远无法提交。
        var mode = _mode;
        _mode = DragMode.None;
        _dragRegion = null;
        RegionCanvas.ReleasePointerCaptures();

        if (mode == DragMode.Draw)
        {
            FinalizeDraw(p);
        }
        else
        {
            // 移动 / 调整结束
            _subscribedVm?.NotifyPackStructureChanged();
            Redraw();
        }
    }

    private void CanvasPointerCanceled(object sender, PointerRoutedEventArgs e)
    {
        // 已由 CanvasReleased 处理时，捕获丢失是释放的正常副产物，直接忽略。
        if (_mode == DragMode.None) { return; }
        _mode = DragMode.None;
        _dragRegion = null;
        RegionCanvas.ReleasePointerCaptures();
        Redraw();
    }

    private void FinalizeDraw(Point end)
    {
        if (_subscribedVm?.SelectedClickImage == null || _imgW <= 0 || _imgH <= 0 || _scale <= 0) { Redraw(); return; }

        var imageLeft = _ox;
        var imageTop = _oy;
        var imageRight = _ox + _imgW * _scale;
        var imageBottom = _oy + _imgH * _scale;

        var x1 = Math.Clamp(Math.Min(_drawStart.X, end.X), imageLeft, imageRight);
        var y1 = Math.Clamp(Math.Min(_drawStart.Y, end.Y), imageTop, imageBottom);
        var x2 = Math.Clamp(Math.Max(_drawStart.X, end.X), imageLeft, imageRight);
        var y2 = Math.Clamp(Math.Max(_drawStart.Y, end.Y), imageTop, imageBottom);
        if (x2 - x1 < 6 || y2 - y1 < 6) { Redraw(); return; }

        var ix1 = Clamp((int)Math.Round((x1 - imageLeft) / _scale), 0, _imgW - 1);
        var iy1 = Clamp((int)Math.Round((y1 - imageTop) / _scale), 0, _imgH - 1);
        var ix2 = Clamp((int)Math.Round((x2 - imageLeft) / _scale), ix1 + 1, _imgW);
        var iy2 = Clamp((int)Math.Round((y2 - imageTop) / _scale), iy1 + 1, _imgH);

        var region = new RegionInfo
        {
            Tag = _activeTag ?? "",
            X = ix1,
            Y = iy1,
            Width = ix2 - ix1,
            Height = iy2 - iy1,
        };
        _subscribedVm.SelectedClickImage.Source.Regions.Add(region);
        _subscribedVm.NotifyPackStructureChanged();
        Select(region);
        Redraw();
    }

    // ─────────────────── 句柄命中 / 调整 ───────────────────

    private const double HandleHitPx = 10;

    private bool HitHandle(RegionInfo r, Point p, out DragMode handle)
    {
        handle = DragMode.None;
        var rect = RegionScreenRect(r);
        var nw = new Point(rect.Left, rect.Top);
        var ne = new Point(rect.Right, rect.Top);
        var sw = new Point(rect.Left, rect.Bottom);
        var se = new Point(rect.Right, rect.Bottom);
        if (Dist(p, se) <= HandleHitPx) { handle = DragMode.ResizeSE; return true; }
        if (Dist(p, ne) <= HandleHitPx) { handle = DragMode.ResizeNE; return true; }
        if (Dist(p, sw) <= HandleHitPx) { handle = DragMode.ResizeSW; return true; }
        if (Dist(p, nw) <= HandleHitPx) { handle = DragMode.ResizeNW; return true; }
        return false;
    }

    private static double Dist(Point a, Point b)
        => Math.Sqrt((a.X - b.X) * (a.X - b.X) + (a.Y - b.Y) * (a.Y - b.Y));

    private void ApplyResize(DragMode mode, Point p)
    {
        var r = _dragRegion;
        if (r == null) return;
        var minDim = Math.Max(4, (int)Math.Ceiling(8 / _scale));

        var start = _dragStartRect;
        var rightEdge = start.X + start.Width;
        var bottomEdge = start.Y + start.Height;

        if (mode is DragMode.ResizeSE or DragMode.ResizeNE)
        {
            var newRight = Clamp((int)Math.Round(ToImageX(p.X)), (int)Math.Round(start.X) + minDim, _imgW);
            r.Width = Math.Max(minDim, newRight - r.X);
        }
        if (mode is DragMode.ResizeSW or DragMode.ResizeNW)
        {
            var newX = Clamp((int)Math.Round(ToImageX(p.X)), 0, (int)Math.Round(rightEdge) - minDim);
            r.Width = Math.Max(minDim, (int)Math.Round(rightEdge) - newX);
            r.X = newX;
        }
        if (mode is DragMode.ResizeSW or DragMode.ResizeSE)
        {
            var newBottom = Clamp((int)Math.Round(ToImageY(p.Y)), (int)Math.Round(start.Y) + minDim, _imgH);
            r.Height = Math.Max(minDim, newBottom - r.Y);
        }
        if (mode is DragMode.ResizeNW or DragMode.ResizeNE)
        {
            var newY = Clamp((int)Math.Round(ToImageY(p.Y)), 0, (int)Math.Round(bottomEdge) - minDim);
            r.Height = Math.Max(minDim, (int)Math.Round(bottomEdge) - newY);
            r.Y = newY;
        }
    }

    // ─────────────────── 绘制 ───────────────────

    private void DrawPreview(Point a, Point b)
    {
        if (_scale <= 0) return;
        var left = Math.Min(a.X, b.X);
        var top = Math.Min(a.Y, b.Y);
        var w = Math.Abs(b.X - a.X);
        var h = Math.Abs(b.Y - a.Y);
        var accent = AccentBrush();
        var rect = new Rectangle
        {
            Width = w,
            Height = h,
            Stroke = accent,
            StrokeThickness = 1.5,
            StrokeDashArray = new DoubleCollection { 3, 3 },
            Fill = BrushWithAlpha(accent.Color, 0x33),
        };
        Canvas.SetLeft(rect, left);
        Canvas.SetTop(rect, top);
        Overlay.Children.Add(rect);
    }

    private void Redraw()
    {
        Overlay.Children.Clear();
        if (_subscribedVm?.SelectedClickImage == null || _scale <= 0) return;

        var regions = _subscribedVm.SelectedClickImage.Source.Regions;
        foreach (var region in regions)
        {
            if (region == null || region.Width <= 0 || region.Height <= 0) continue;
            DrawRegion(region, ReferenceEquals(region, _selected));
        }
    }

    private void DrawRegion(RegionInfo region, bool selected)
    {
        var color = GetTagColor(region.Tag);
        var x1 = Clamp(region.X, 0, _imgW);
        var y1 = Clamp(region.Y, 0, _imgH);
        var x2 = (int)Math.Clamp((long)region.X + region.Width, 0L, _imgW);
        var y2 = (int)Math.Clamp((long)region.Y + region.Height, 0L, _imgH);
        if (x2 <= x1 || y2 <= y1) return;

        var pos = ToScreen(x1, y1);
        var w = (x2 - x1) * _scale;
        var h = (y2 - y1) * _scale;

        var rect = new Rectangle
        {
            Stroke = color,
            StrokeThickness = selected ? 2.5 : 1.75,
            Fill = BrushWithAlpha(color.Color, (byte)(selected ? 0x55 : 0x33)),
            RadiusX = 4,
            RadiusY = 4,
            Width = w,
            Height = h,
        };
        Canvas.SetLeft(rect, pos.X);
        Canvas.SetTop(rect, pos.Y);
        Overlay.Children.Add(rect);

        // 标签 chip
        var tagText = string.IsNullOrWhiteSpace(region.Tag) ? "未标记" : region.Tag;
        var chip = new Border
        {
            Background = color,
            CornerRadius = new CornerRadius(3),
            Padding = new Thickness(5, 1, 5, 1),
            Child = new TextBlock
            {
                Text = tagText,
                FontSize = 10,
                Foreground = new SolidColorBrush(Colors.White),
            },
        };
        var chipTop = Math.Max(0, pos.Y - 16);
        Canvas.SetLeft(chip, pos.X);
        Canvas.SetTop(chip, chipTop);
        Overlay.Children.Add(chip);

        // 选中句柄
        if (selected)
        {
            DrawHandle(pos.X, pos.Y);
            DrawHandle(pos.X + w, pos.Y);
            DrawHandle(pos.X, pos.Y + h);
            DrawHandle(pos.X + w, pos.Y + h);
        }
    }

    private void DrawHandle(double x, double y)
    {
        const double size = 9;
        var h = new Rectangle
        {
            Width = size,
            Height = size,
            Fill = new SolidColorBrush(Colors.White),
            Stroke = AccentBrush(),
            StrokeThickness = 1.5,
            RadiusX = 2,
            RadiusY = 2,
        };
        Canvas.SetLeft(h, x - size / 2);
        Canvas.SetTop(h, y - size / 2);
        Overlay.Children.Add(h);
    }

    // ─────────────────── 配色 ───────────────────

    private SolidColorBrush GetTagColor(string? tag)
    {
        var key = tag ?? "";
        if (_tagColors.TryGetValue(key, out var b)) return b;
        // 兜底：分配一个调色板色
        var c = Palette[_tagColors.Count % Palette.Length];
        var brush = new SolidColorBrush(c);
        _tagColors[key] = brush;
        return brush;
    }

    private static SolidColorBrush BrushWithAlpha(Windows.UI.Color c, byte alpha)
    {
        var copy = c;
        copy.A = alpha;
        return new SolidColorBrush(copy);
    }

    private SolidColorBrush AccentBrush()
    {
        if (Application.Current.Resources.TryGetValue("AccentFillColorDefaultBrush", out var o) && o is SolidColorBrush b)
            return b;
        return new SolidColorBrush(Windows.UI.Color.FromArgb(0xFF, 0x00, 0x78, 0xD4));
    }

    // ─────────────────── 分组 / 选中 ───────────────────

    private void RebuildGroups()
    {
        // 解除旧行的 Source 事件订阅，避免每次重建泄漏（RegionRow 构造时订阅）
        foreach (var g in RegionGroups)
            foreach (var row in g.Rows)
                row.Detach();

        var src = _subscribedVm?.SelectedClickImage;
        var regions = src?.Source.Regions;

        // 重新分配标签配色（保留已有颜色，新增的按调色板补）
        _tagColors.Clear();
        if (regions != null)
        {
            int ci = 0;
            foreach (var r in regions)
            {
                if (r == null) continue;
                var key = r.Tag ?? "";
                if (!_tagColors.ContainsKey(key))
                {
                    _tagColors[key] = new SolidColorBrush(Palette[ci % Palette.Length]);
                    ci++;
                }
            }
        }

        RegionGroups.Clear();
        if (regions == null) { RegionCount = 0; return; }

        var byTag = new Dictionary<string, RegionGroup>();
        var ordered = new List<RegionGroup>();
        foreach (var r in regions)
        {
            if (r == null) continue;
            var key = r.Tag ?? "";
            if (!byTag.TryGetValue(key, out var g))
            {
                g = new RegionGroup(key, _tagColors[key]);
                byTag[key] = g;
                ordered.Add(g);
            }
            var row = new RegionRow(r, _tagColors[key], BrushWithAlpha(_tagColors[key].Color, 0x33), _tagColors[key]);
            g.Rows.Add(row);
        }

        foreach (var g in ordered)
        {
            g.IsActive = (_activeTag != null && g.Tag == _activeTag);
            RegionGroups.Add(g);
        }

        RegionCount = regions.Count;
        UpdateRowSelection();
        UpdateActiveHint();
    }

    private void Select(RegionInfo? region)
    {
        _selected = region;
        UpdateRowSelection();
        Redraw();
    }

    private void UpdateRowSelection()
    {
        foreach (var g in RegionGroups)
            foreach (var row in g.Rows)
                row.IsSelected = ReferenceEquals(row.Source, _selected);
    }

    private void UpdateActiveHint()
    {
        if (_activeTag == null)
        {
            ActiveTagHint.Visibility = Visibility.Collapsed;
            return;
        }
        ActiveTagHint.Visibility = Visibility.Visible;
        ActiveTagHint.Text = "绘制目标：" + (string.IsNullOrEmpty(_activeTag) ? "未标记" : _activeTag);
    }

    // ─────────────────── 列表事件 ───────────────────

    private void SelectRegion(object sender, TappedRoutedEventArgs e)
    {
        if (sender is FrameworkElement fe && fe.Tag is RegionInfo r)
        {
            Select(r);
        }
    }

    private void DeleteRegion(object sender, RoutedEventArgs e)
    {
        if (sender is Button { Tag: RegionInfo region } && _subscribedVm?.SelectedClickImage != null)
        {
            if (ReferenceEquals(region, _selected)) _selected = null;
            _subscribedVm.SelectedClickImage.Source.Regions.Remove(region);
            _subscribedVm.NotifyPackStructureChanged();
            // RegionsCollectionChanged 会重建分组并重绘
        }
    }

    private void DeleteGroup(object sender, RoutedEventArgs e)
    {
        if (sender is Button { Tag: RegionGroup g } && _subscribedVm?.SelectedClickImage != null)
        {
            var regions = _subscribedVm.SelectedClickImage.Source.Regions;
            for (int i = regions.Count - 1; i >= 0; i--)
            {
                if (regions[i] is { } r && (r.Tag ?? "") == g.Tag)
                    regions.RemoveAt(i);
            }
            if (g.IsActive) _activeTag = null;
            _subscribedVm.NotifyPackStructureChanged();
            // RegionsCollectionChanged 重建并重绘
        }
    }

    private void ToggleActiveGroup(object sender, RoutedEventArgs e)
    {
        if (sender is ToggleButton tb && tb.DataContext is RegionGroup g)
        {
            _activeTag = g.IsActive ? g.Tag : null;
            foreach (var gg in RegionGroups)
                gg.IsActive = (gg.Tag == _activeTag && _activeTag != null);
            UpdateActiveHint();
        }
    }

    private void ClearAll(object sender, RoutedEventArgs e)
    {
        if (_subscribedVm?.SelectedClickImage == null
            || _subscribedVm.SelectedClickImage.Source.Regions.Count == 0) return;
        _selected = null;
        _subscribedVm.SelectedClickImage.Source.Regions.Clear();
        _subscribedVm.NotifyPackStructureChanged();
        // RegionsCollectionChanged 重建并重绘
    }

    // ─────────────────── 标签编辑 ───────────────────
    // 区域标签改用 TagEditor（单选），无需 AutoSuggestBox 的 Loaded/TextChanged/LostFocus 处理；
    // 标签提交后由 RegionPropertyChanged(Tag) 延迟重建分组。

    // ─────────────────── 工具 ───────────────────

    private static int Clamp(int value, int min, int max)
        => value < min ? min : value > max ? max : value;
}
