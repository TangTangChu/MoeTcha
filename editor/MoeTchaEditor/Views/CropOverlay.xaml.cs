using Microsoft.UI;
using Microsoft.UI.Input;
using Microsoft.UI.Text;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Input;
using Microsoft.UI.Xaml.Media;
using Microsoft.UI.Xaml.Media.Imaging;
using Microsoft.UI.Xaml.Shapes;
using Windows.Foundation;
using MoeTchaEditor.Services;

namespace MoeTchaEditor.Views;

public sealed partial class CropOverlay : UserControl
{
    private string _sourcePath = "";
    private string _outputDir = "";
    private string _baseFileName = "";

    private double _scale = 1;
    private double _ox, _oy;
    private int _imgW, _imgH;
    private int _maxCropSize;
    private int _cropSize;
    private int _cropX, _cropY;

    private bool _dragging;
    private DragMode _dragMode = DragMode.None;
    private Point _dragOrigin;
    private int _dragStartX, _dragStartY;
    private int _dragStartSize;
    private const double EdgeThreshold = 12;

    private static readonly SolidColorBrush MaskBrush = new(Windows.UI.Color.FromArgb(0xA0, 0, 0, 0));
    private static readonly SolidColorBrush HandleFill = new(Windows.UI.Color.FromArgb(0xFF, 0xFF, 0xFF, 0xFF));
    private const double HandleSz = 9;

    // 持久化绘制元素：Redraw 只更新位置/尺寸，避免每次 PointerMoved 重建元素造成的卡顿
    private Rectangle? _maskTop, _maskBottom, _maskLeft, _maskRight;
    private Rectangle? _frame;
    private SolidColorBrush? _frameFill;
    private Rectangle?[] _handles = new Rectangle?[4];
    private Border? _chip;
    private TextBlock? _chipText;
    private bool _shapesReady;

    private TaskCompletionSource<string?> _tcs = CreateCompletionSource();

    private enum DragMode { None, Move, N, S, E, W, NW, NE, SW, SE }

    public CropOverlay()
    {
        InitializeComponent();
    }

    public Task<string?> Completion => _tcs.Task;

    public void Begin(string sourcePath, string outputDir, string baseFileName)
    {
        _tcs.TrySetResult(null);
        _tcs = CreateCompletionSource();

        _sourcePath = sourcePath;
        _outputDir = outputDir;
        _baseFileName = baseFileName;
        _imgW = _imgH = _maxCropSize = _cropSize = _cropX = _cropY = 0;
        _scale = 1;
        _ox = _oy = 0;
        _dragging = false;
        _dragMode = DragMode.None;
        CropFileName.Text = System.IO.Path.GetFileName(baseFileName);
        SizeLabel.Text = "";
        OverlayCanvas.Children.Clear();
        _shapesReady = false;

        try
        {
            var bmp = new BitmapImage { UriSource = new Uri(sourcePath, UriKind.Absolute) };
            Preview.Source = bmp;
        }
        catch
        {
            Complete(null);
        }
    }

    public void Cancel() => Complete(null);

    private void OnImageOpened(object sender, RoutedEventArgs e)
    {
        try
        {
            if (Preview.Source is not BitmapSource b || b.PixelWidth == 0 || b.PixelHeight == 0)
            {
                Complete(null);
                return;
            }

            using var sk = SkiaSharp.SKBitmap.Decode(_sourcePath);
            if (sk == null)
            {
                Complete(null);
                return;
            }

            _imgW = sk.Width;
            _imgH = sk.Height;
            _maxCropSize = Math.Min(_imgW, _imgH);
            _cropSize = _maxCropSize;
            _cropX = (_imgW - _cropSize) / 2;
            _cropY = (_imgH - _cropSize) / 2;

            SizeSlider.Value = 100;
            UpdateSizeLabel();
            UpdateImageGeometry();
        }
        catch
        {
            Complete(null);
        }
    }

    private void OnImageFailed(object sender, ExceptionRoutedEventArgs e)
        => Complete(null);

    private void OnLayoutChanged(object sender, SizeChangedEventArgs e)
        => UpdateImageGeometry();

    private void UpdateImageGeometry()
    {
        if (_imgW <= 0 || _imgH <= 0 || OverlayCanvas.ActualWidth <= 0 || OverlayCanvas.ActualHeight <= 0)
            return;

        _scale = Math.Min(OverlayCanvas.ActualWidth / _imgW, OverlayCanvas.ActualHeight / _imgH);
        _ox = (OverlayCanvas.ActualWidth - _imgW * _scale) / 2;
        _oy = (OverlayCanvas.ActualHeight - _imgH * _scale) / 2;
        Redraw();
    }

    // ──────────── hit test ────────────

    private DragMode HitTest(Point pos)
    {
        var left = _ox + _cropX * _scale;
        var top = _oy + _cropY * _scale;
        var sz = _cropSize * _scale;
        var right = left + sz;
        var bottom = top + sz;

        var nearL = Math.Abs(pos.X - left) < EdgeThreshold;
        var nearR = Math.Abs(pos.X - right) < EdgeThreshold;
        var nearT = Math.Abs(pos.Y - top) < EdgeThreshold;
        var nearB = Math.Abs(pos.Y - bottom) < EdgeThreshold;
        var inside = pos.X > left + EdgeThreshold && pos.X < right - EdgeThreshold
                  && pos.Y > top + EdgeThreshold && pos.Y < bottom - EdgeThreshold;

        if (nearR && nearB) return DragMode.SE;
        if (nearR && nearT) return DragMode.NE;
        if (nearL && nearB) return DragMode.SW;
        if (nearL && nearT) return DragMode.NW;
        if (nearR && pos.Y >= top && pos.Y <= bottom) return DragMode.E;
        if (nearL && pos.Y >= top && pos.Y <= bottom) return DragMode.W;
        if (nearB && pos.X >= left && pos.X <= right) return DragMode.S;
        if (nearT && pos.X >= left && pos.X <= right) return DragMode.N;
        if (inside) return DragMode.Move;
        return DragMode.None;
    }

    private static InputCursor? CursorFor(DragMode mode) => mode switch
    {
        DragMode.NW or DragMode.SE => InputSystemCursor.Create(InputSystemCursorShape.SizeNorthwestSoutheast),
        DragMode.NE or DragMode.SW => InputSystemCursor.Create(InputSystemCursorShape.SizeNortheastSouthwest),
        DragMode.N or DragMode.S => InputSystemCursor.Create(InputSystemCursorShape.SizeNorthSouth),
        DragMode.E or DragMode.W => InputSystemCursor.Create(InputSystemCursorShape.SizeWestEast),
        DragMode.Move => InputSystemCursor.Create(InputSystemCursorShape.SizeAll),
        _ => null,
    };

    // ──────────── pointer handlers ────────────

    private void OverlayPressed(object sender, PointerRoutedEventArgs e)
    {
        if (_imgW <= 0 || _imgH <= 0 || _scale <= 0) return;
        _dragMode = HitTest(e.GetCurrentPoint(OverlayCanvas).Position);
        if (_dragMode == DragMode.None) return;

        _dragging = true;
        _dragOrigin = e.GetCurrentPoint(OverlayCanvas).Position;
        _dragStartX = _cropX;
        _dragStartY = _cropY;
        _dragStartSize = _cropSize;
        OverlayCanvas.CapturePointer(e.Pointer);
        e.Handled = true;
    }

    private void OverlayMoved(object sender, PointerRoutedEventArgs e)
    {
        var pos = e.GetCurrentPoint(OverlayCanvas).Position;

        if (_dragging)
        {
            ApplyDrag(pos);
            Redraw();
            e.Handled = true;
            return;
        }

        // hover cursor
        ProtectedCursor = CursorFor(HitTest(pos));
    }

    private void ApplyDrag(Point pos)
    {
        var dx = (pos.X - _dragOrigin.X) / _scale;
        var dy = (pos.Y - _dragOrigin.Y) / _scale;

        switch (_dragMode)
        {
            case DragMode.Move:
                _cropX = Clamp(_dragStartX + (int)Math.Round(dx), 0, _imgW - _cropSize);
                _cropY = Clamp(_dragStartY + (int)Math.Round(dy), 0, _imgH - _cropSize);
                break;

            case DragMode.E:
                ResizeFromEast(dx);
                break;
            case DragMode.W:
                ResizeFromWest(dx);
                break;
            case DragMode.S:
                ResizeFromSouth(dy);
                break;
            case DragMode.N:
                ResizeFromNorth(dy);
                break;

            case DragMode.SE:
                ResizeFromCorner(dx, dy, fixedLeft: true, fixedTop: true);
                break;
            case DragMode.NE:
                ResizeFromCorner(dx, dy, fixedLeft: true, fixedBottom: true);
                break;
            case DragMode.SW:
                ResizeFromCorner(dx, dy, fixedRight: true, fixedTop: true);
                break;
            case DragMode.NW:
                ResizeFromCorner(dx, dy, fixedRight: true, fixedBottom: true);
                break;
        }

        SyncSlider();
        UpdateSizeLabel();
    }

    private void ResizeFromEast(double dx)
    {
        var newRight = _dragStartX + _dragStartSize + (int)Math.Round(dx);
        var newSize = Clamp(newRight - _dragStartX, 1, Math.Min(_imgW - _dragStartX, _imgH - _dragStartY));
        _cropSize = newSize;
        _cropX = _dragStartX;
        _cropY = _dragStartY;
    }

    private void ResizeFromWest(double dx)
    {
        var origRight = _dragStartX + _dragStartSize;
        var newLeft = _dragStartX + (int)Math.Round(dx);
        var maxSize = Math.Min(origRight, Math.Min(_imgH - _dragStartY, origRight));
        var newSize = Clamp(origRight - newLeft, 1, Math.Min(origRight, _imgH - _dragStartY));
        _cropSize = newSize;
        _cropX = origRight - newSize;
        _cropY = _dragStartY;
    }

    private void ResizeFromSouth(double dy)
    {
        var newBottom = _dragStartY + _dragStartSize + (int)Math.Round(dy);
        var newSize = Clamp(newBottom - _dragStartY, 1, Math.Min(_imgW - _dragStartX, _imgH - _dragStartY));
        _cropSize = newSize;
        _cropX = _dragStartX;
        _cropY = _dragStartY;
    }

    private void ResizeFromNorth(double dy)
    {
        var origBottom = _dragStartY + _dragStartSize;
        var newTop = _dragStartY + (int)Math.Round(dy);
        var newSize = Clamp(origBottom - newTop, 1, Math.Min(_imgW - _dragStartX, _imgH - _dragStartY));
        _cropSize = newSize;
        _cropX = _dragStartX;
        _cropY = origBottom - newSize;
    }

    private void ResizeFromCorner(double dx, double dy,
        bool fixedLeft = false, bool fixedTop = false, bool fixedRight = false, bool fixedBottom = false)
    {
        // 以拖拽起点矩形为基准，按鼠标增量计算新的边长，
        // 这样重新拖动时尺寸从当前大小连续变化，不会从 0 重置。
        var startLeft = _dragStartX;
        var startTop = _dragStartY;
        var startRight = _dragStartX + _dragStartSize;
        var startBottom = _dragStartY + _dragStartSize;
        var startSize = _dragStartSize;
        var idx = (int)Math.Round(dx);
        var idy = (int)Math.Round(dy);

        int s, x, y;
        if (fixedLeft && fixedTop) // SE：固定左上角
        {
            s = Math.Max(startSize + idx, startSize + idy);
            s = Clamp(s, 1, Math.Min(_imgW - startLeft, _imgH - startTop));
            x = startLeft; y = startTop;
        }
        else if (fixedLeft && fixedBottom) // NE：固定左下角
        {
            s = Math.Max(startSize + idx, startSize - idy);
            s = Clamp(s, 1, Math.Min(_imgW - startLeft, startBottom));
            x = startLeft; y = startBottom - s;
        }
        else if (fixedRight && fixedTop) // SW：固定右上角
        {
            s = Math.Max(startSize - idx, startSize + idy);
            s = Clamp(s, 1, Math.Min(startRight, _imgH - startTop));
            x = startRight - s; y = startTop;
        }
        else // NW：固定右下角
        {
            s = Math.Max(startSize - idx, startSize - idy);
            s = Clamp(s, 1, Math.Min(startRight, startBottom));
            x = startRight - s; y = startBottom - s;
        }

        _cropSize = s;
        _cropX = Clamp(x, 0, _imgW - s);
        _cropY = Clamp(y, 0, _imgH - s);
    }

    private void SyncSlider()
    {
        if (_maxCropSize <= 0) return;
        var pct = Math.Round(_cropSize * 100.0 / _maxCropSize);
        SizeSlider.Value = Clamp((int)pct, 1, 100);
    }

    private void OverlayReleased(object sender, PointerRoutedEventArgs e)
    {
        _dragging = false;
        _dragMode = DragMode.None;
        OverlayCanvas.ReleasePointerCaptures();
        ProtectedCursor = null;
        e.Handled = true;
    }

    private void OverlayPointerCanceled(object sender, PointerRoutedEventArgs e)
    {
        _dragging = false;
        _dragMode = DragMode.None;
        OverlayCanvas.ReleasePointerCaptures();
        ProtectedCursor = null;
    }

    // ──────────── mouse wheel resize ────────────

    private void OverlayWheel(object sender, PointerRoutedEventArgs e)
    {
        if (_imgW <= 0 || _imgH <= 0 || _maxCropSize <= 0 || _dragging) return;

        var props = e.GetCurrentPoint(OverlayCanvas).Properties;
        var delta = props.MouseWheelDelta;
        if (delta == 0) return;

        // Keep centre; grow/shrink by 2% of max per notch
        var step = Math.Max(1, (int)Math.Round(_maxCropSize * 0.02));
        var change = delta > 0 ? step : -step;

        var centerX = _cropX + _cropSize / 2.0;
        var centerY = _cropY + _cropSize / 2.0;
        var newSize = Clamp(_cropSize + change, 1, _maxCropSize);
        _cropSize = newSize;
        _cropX = Clamp((int)Math.Round(centerX - newSize / 2.0), 0, _imgW - newSize);
        _cropY = Clamp((int)Math.Round(centerY - newSize / 2.0), 0, _imgH - newSize);

        SyncSlider();
        UpdateSizeLabel();
        Redraw();
        e.Handled = true;
    }

    // ──────────── slider ────────────

    private void OnSizeChanged(object sender, Microsoft.UI.Xaml.Controls.Primitives.RangeBaseValueChangedEventArgs e)
    {
        if (_maxCropSize == 0 || _dragging) return;

        var centerX = _cropX + _cropSize / 2.0;
        var centerY = _cropY + _cropSize / 2.0;
        _cropSize = Math.Max(1, (int)Math.Round(_maxCropSize * e.NewValue / 100));
        _cropX = Clamp((int)Math.Round(centerX - _cropSize / 2.0), 0, _imgW - _cropSize);
        _cropY = Clamp((int)Math.Round(centerY - _cropSize / 2.0), 0, _imgH - _cropSize);
        UpdateSizeLabel();
        Redraw();
    }

    private void UpdateSizeLabel() => SizeLabel.Text = $"{_cropSize}×{_cropSize}";

    // ──────────── confirm / skip ────────────

    private async void OnConfirm(object sender, RoutedEventArgs e)
    {
        if (_imgW <= 0 || _imgH <= 0 || _cropSize <= 0)
        {
            Complete(null);
            return;
        }

        var completion = _tcs;
        try
        {
            ConfirmBtn.IsEnabled = false;
            SkipBtn.IsEnabled = false;
            SizeSlider.IsEnabled = false;
            OverlayCanvas.IsHitTestVisible = false;
            var sourcePath = _sourcePath;
            var outputDir = _outputDir;
            var baseFileName = _baseFileName;
            var cropX = _cropX;
            var cropY = _cropY;
            var cropSize = _cropSize;
            var outName = await Task.Run(() => ImageProcessor.CropSquareAndWebp(
                sourcePath, outputDir, baseFileName, cropX, cropY, cropSize));
            if (ReferenceEquals(_tcs, completion)) Complete(outName);
        }
        catch
        {
            if (ReferenceEquals(_tcs, completion)) Complete(null);
        }
        finally
        {
            ConfirmBtn.IsEnabled = true;
            SkipBtn.IsEnabled = true;
            SizeSlider.IsEnabled = true;
            OverlayCanvas.IsHitTestVisible = true;
        }
    }

    private void OnSkip(object sender, RoutedEventArgs e)
        => Complete(null);

    private void Complete(string? result)
    {
        _dragging = false;
        _dragMode = DragMode.None;
        OverlayCanvas.ReleasePointerCaptures();
        ProtectedCursor = null;
        _tcs.TrySetResult(result);
        Visibility = Visibility.Collapsed;
    }

    // ──────────── drawing ────────────

    private void Redraw()
    {
        EnsureShapes();
        if (_imgW <= 0 || _imgH <= 0 || _scale <= 0 || _frame == null)
        {
            HideShapes();
            return;
        }

        var cw = OverlayCanvas.ActualWidth;
        var ch = OverlayCanvas.ActualHeight;
        var left = _ox + _cropX * _scale;
        var top = _oy + _cropY * _scale;
        var w = _cropSize * _scale;
        var h = _cropSize * _scale;

        // masks (dark scrim outside the crop square) - 仅更新位置/尺寸，不重建元素
        PositionMask(_maskTop!, 0, 0, cw, top);
        PositionMask(_maskBottom!, 0, top + h, cw, ch - top - h);
        PositionMask(_maskLeft!, 0, top, left, h);
        PositionMask(_maskRight!, left + w, top, cw - left - w, h);

        // frame
        _frame.Width = Math.Max(0, w);
        _frame.Height = Math.Max(0, h);
        Canvas.SetLeft(_frame, left);
        Canvas.SetTop(_frame, top);
        _frame.Visibility = Visibility.Visible;

        // size chip at top-left
        _chipText!.Text = $"{_cropSize}×{_cropSize}";
        Canvas.SetLeft(_chip!, left);
        Canvas.SetTop(_chip, Math.Max(2, top - 22));
        _chip!.Visibility = Visibility.Visible;

        // corner handles
        SetHandle(_handles[0]!, left, top);
        SetHandle(_handles[1]!, left + w, top);
        SetHandle(_handles[2]!, left, top + h);
        SetHandle(_handles[3]!, left + w, top + h);
    }

    private void EnsureShapes()
    {
        if (_shapesReady) return;
        var accent = AccentBrush();
        _frameFill = new SolidColorBrush(Windows.UI.Color.FromArgb(0x22, accent.Color.R, accent.Color.G, accent.Color.B));

        _maskTop = new Rectangle { Fill = MaskBrush };
        _maskBottom = new Rectangle { Fill = MaskBrush };
        _maskLeft = new Rectangle { Fill = MaskBrush };
        _maskRight = new Rectangle { Fill = MaskBrush };

        _frame = new Rectangle
        {
            Stroke = accent,
            StrokeThickness = 2,
            Fill = _frameFill,
            RadiusX = 6,
            RadiusY = 6,
        };

        _handles = new Rectangle[4];
        for (int i = 0; i < 4; i++)
            _handles[i] = new Rectangle
            {
                Fill = HandleFill,
                Stroke = accent,
                StrokeThickness = 1.5,
                RadiusX = 2,
                RadiusY = 2,
                Width = HandleSz,
                Height = HandleSz,
            };

        _chipText = new TextBlock
        {
            FontSize = 12,
            FontWeight = FontWeights.SemiBold,
            Foreground = new SolidColorBrush(Windows.UI.Color.FromArgb(0xFF, 0xFF, 0xFF, 0xFF)),
        };
        _chip = new Border
        {
            Background = accent,
            CornerRadius = new CornerRadius(4),
            Padding = new Thickness(7, 2, 7, 2),
            Child = _chipText,
        };

        foreach (var m in new[] { _maskTop, _maskBottom, _maskLeft, _maskRight })
            OverlayCanvas.Children.Add(m);
        OverlayCanvas.Children.Add(_frame);
        foreach (var hd in _handles)
            OverlayCanvas.Children.Add(hd);
        OverlayCanvas.Children.Add(_chip);

        _shapesReady = true;
    }

    private static void PositionMask(Rectangle r, double x, double y, double w, double h)
    {
        if (w <= 0 || h <= 0) { r.Visibility = Visibility.Collapsed; return; }
        r.Width = w;
        r.Height = h;
        Canvas.SetLeft(r, x);
        Canvas.SetTop(r, y);
        r.Visibility = Visibility.Visible;
    }

    private void SetHandle(Rectangle r, double x, double y)
    {
        Canvas.SetLeft(r, x - HandleSz / 2);
        Canvas.SetTop(r, y - HandleSz / 2);
        r.Visibility = Visibility.Visible;
    }

    private void HideShapes()
    {
        foreach (var m in new[] { _maskTop, _maskBottom, _maskLeft, _maskRight })
            if (m != null) m.Visibility = Visibility.Collapsed;
        if (_frame != null) _frame.Visibility = Visibility.Collapsed;
        if (_chip != null) _chip.Visibility = Visibility.Collapsed;
        foreach (var hd in _handles)
            if (hd != null) hd.Visibility = Visibility.Collapsed;
    }

    private SolidColorBrush AccentBrush()
    {
        if (Application.Current.Resources.TryGetValue("AccentFillColorDefaultBrush", out var o) && o is SolidColorBrush b)
            return b;
        return new SolidColorBrush(Windows.UI.Color.FromArgb(0xFF, 0x00, 0x78, 0xD4));
    }

    private static int Clamp(int v, int min, int max) => v < min ? min : v > max ? max : v;

    private static TaskCompletionSource<string?> CreateCompletionSource()
        => new(TaskCreationOptions.RunContinuationsAsynchronously);
}
