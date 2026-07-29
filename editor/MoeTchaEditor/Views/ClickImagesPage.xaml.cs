using System.ComponentModel;
using Microsoft.UI;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Input;
using Microsoft.UI.Xaml.Media;
using Microsoft.UI.Xaml.Media.Imaging;
using Microsoft.UI.Xaml.Shapes;
using Windows.Foundation;
using MoeTchaEditor.Models;
using MoeTchaEditor.ViewModels;

namespace MoeTchaEditor.Views;

public sealed partial class ClickImagesPage : Page
{
    private EditorViewModel? _subscribedVm;
    private PropertyChangedEventHandler? _vmPropertyChanged;
    private readonly List<RegionInfo> _observedRegions = [];

    private double _scale;
    private double _ox, _oy;
    private int _imgW, _imgH;
    private Point _start;
    private bool _drawing;

    // x:Bind 可以在页面加载和卸载的边界被调用。这里必须是无副作用的，
    // 不能用 getter 顺便改变事件订阅生命周期。
    private EditorViewModel? VM => DataContext as EditorViewModel;

    public ClickImagesPage()
    {
        InitializeComponent();
        DataContextChanged += OnDataContextChanged;
        Loaded += OnLoaded;
        Unloaded += OnUnloaded;
    }

    private void OnDataContextChanged(FrameworkElement sender, DataContextChangedEventArgs args)
    {
        if (IsLoaded) AttachViewModel();
    }

    private void OnLoaded(object sender, RoutedEventArgs e)
        => AttachViewModel();

    private void OnUnloaded(object sender, RoutedEventArgs e)
    {
        _drawing = false;
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
            if (args.PropertyName is nameof(EditorViewModel.SelectedClickImage)
                or nameof(EditorViewModel.ClickImageDisplays))
            {
                AttachRegionHandlers();
                ClearGeometry();
                Redraw();
            }
        };
        _subscribedVm.PropertyChanged += _vmPropertyChanged;
        AttachRegionHandlers();
    }

    private void DetachViewModel()
    {
        if (_subscribedVm != null && _vmPropertyChanged != null)
            _subscribedVm.PropertyChanged -= _vmPropertyChanged;
        _vmPropertyChanged = null;
        _subscribedVm = null;
        DetachRegionHandlers();
    }

    private void AttachRegionHandlers()
    {
        DetachRegionHandlers();
        if (_subscribedVm?.SelectedClickImage == null) return;

        foreach (var region in _subscribedVm.SelectedClickImage.Source.Regions)
        {
            if (region == null) continue;
            region.PropertyChanged += RegionPropertyChanged;
            _observedRegions.Add(region);
        }
    }

    private void DetachRegionHandlers()
    {
        foreach (var region in _observedRegions)
            region.PropertyChanged -= RegionPropertyChanged;
        _observedRegions.Clear();
    }

    private void RegionPropertyChanged(object? sender, PropertyChangedEventArgs e)
    {
        if (e.PropertyName is nameof(RegionInfo.Tag) or nameof(RegionInfo.X)
            or nameof(RegionInfo.Y) or nameof(RegionInfo.Width) or nameof(RegionInfo.Height))
            Redraw();
    }

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

    private void CanvasPressed(object sender, PointerRoutedEventArgs e)
    {
        if (_subscribedVm?.SelectedClickImage == null || _imgW <= 0 || _imgH <= 0 || _scale <= 0) return;

        _drawing = true;
        _start = e.GetCurrentPoint(RegionCanvas).Position;
        RegionCanvas.CapturePointer(e.Pointer);
        e.Handled = true;
    }

    private void CanvasReleased(object sender, PointerRoutedEventArgs e)
    {
        if (!_drawing) return;

        _drawing = false;
        RegionCanvas.ReleasePointerCaptures();
        e.Handled = true;

        if (_subscribedVm?.SelectedClickImage == null || _imgW <= 0 || _imgH <= 0 || _scale <= 0) return;

        var end = e.GetCurrentPoint(RegionCanvas).Position;
        var imageLeft = _ox;
        var imageTop = _oy;
        var imageRight = _ox + _imgW * _scale;
        var imageBottom = _oy + _imgH * _scale;

        var x1 = Math.Clamp(Math.Min(_start.X, end.X), imageLeft, imageRight);
        var y1 = Math.Clamp(Math.Min(_start.Y, end.Y), imageTop, imageBottom);
        var x2 = Math.Clamp(Math.Max(_start.X, end.X), imageLeft, imageRight);
        var y2 = Math.Clamp(Math.Max(_start.Y, end.Y), imageTop, imageBottom);
        if (x2 - x1 < 8 || y2 - y1 < 8) return;

        var ix1 = Clamp((int)Math.Round((x1 - imageLeft) / _scale), 0, _imgW - 1);
        var iy1 = Clamp((int)Math.Round((y1 - imageTop) / _scale), 0, _imgH - 1);
        var ix2 = Clamp((int)Math.Round((x2 - imageLeft) / _scale), ix1 + 1, _imgW);
        var iy2 = Clamp((int)Math.Round((y2 - imageTop) / _scale), iy1 + 1, _imgH);

        _subscribedVm.SelectedClickImage.Source.Regions.Add(new RegionInfo
        {
            Tag = "",
            X = ix1,
            Y = iy1,
            Width = ix2 - ix1,
            Height = iy2 - iy1,
        });
        _subscribedVm.NotifyPackStructureChanged();
        AttachRegionHandlers();
        Redraw();
    }

    private void CanvasPointerCanceled(object sender, PointerRoutedEventArgs e)
    {
        _drawing = false;
        RegionCanvas.ReleasePointerCaptures();
    }

    private void Redraw()
    {
        Overlay.Children.Clear();
        if (_subscribedVm?.SelectedClickImage == null || _scale <= 0) return;

        foreach (var region in _subscribedVm.SelectedClickImage.Source.Regions)
        {
            if (region == null || region.Width <= 0 || region.Height <= 0) continue;

            var x1 = Clamp(region.X, 0, _imgW);
            var y1 = Clamp(region.Y, 0, _imgH);
            var x2 = (int)Math.Clamp((long)region.X + region.Width, 0L, _imgW);
            var y2 = (int)Math.Clamp((long)region.Y + region.Height, 0L, _imgH);
            if (x2 <= x1 || y2 <= y1) continue;

            var left = _ox + x1 * _scale;
            var top = _oy + y1 * _scale;
            var width = (x2 - x1) * _scale;
            var height = (y2 - y1) * _scale;
            var rect = new Rectangle
            {
                Stroke = new SolidColorBrush(Colors.Lime),
                StrokeThickness = 2,
                Fill = new SolidColorBrush(new Windows.UI.Color { A = 60, R = 0, G = 255, B = 0 }),
                Width = width,
                Height = height,
            };
            Canvas.SetLeft(rect, left);
            Canvas.SetTop(rect, top);
            Overlay.Children.Add(rect);

            if (!string.IsNullOrWhiteSpace(region.Tag))
            {
                var text = new TextBlock
                {
                    Text = region.Tag,
                    FontSize = 11,
                    Foreground = new SolidColorBrush(Colors.Lime),
                };
                Canvas.SetLeft(text, left + 2);
                Canvas.SetTop(text, top + 2);
                Overlay.Children.Add(text);
            }
        }
    }

    private void DeleteRegion(object sender, RoutedEventArgs e)
    {
        if (sender is Button { Tag: RegionInfo region } && _subscribedVm?.SelectedClickImage != null)
        {
            _subscribedVm.SelectedClickImage.Source.Regions.Remove(region);
            _subscribedVm.NotifyPackStructureChanged();
            AttachRegionHandlers();
            Redraw();
        }
    }

    private void ClearAll(object sender, RoutedEventArgs e)
    {
        if (_subscribedVm?.SelectedClickImage == null
            || _subscribedVm.SelectedClickImage.Source.Regions.Count == 0) return;
        _subscribedVm.SelectedClickImage.Source.Regions.Clear();
        _subscribedVm.NotifyPackStructureChanged();
        AttachRegionHandlers();
        Redraw();
    }

    private static int Clamp(int value, int min, int max)
        => value < min ? min : value > max ? max : value;

    private void RegionTagBoxLoaded(object sender, RoutedEventArgs e)
    {
        if (sender is AutoSuggestBox box && VM != null)
            box.ItemsSource = VM.TagKeys;
    }

    private void RegionTagBoxTextChanged(AutoSuggestBox sender, AutoSuggestBoxTextChangedEventArgs args)
    {
        if (args.Reason != AutoSuggestionBoxTextChangeReason.UserInput) return;
        if (VM == null) return;

        var query = (sender.Text ?? "").Trim();
        sender.ItemsSource = string.IsNullOrWhiteSpace(query)
            ? VM.TagKeys
            : VM.TagKeys.Where(k => k.Contains(query, StringComparison.OrdinalIgnoreCase)).ToList();
    }
}
