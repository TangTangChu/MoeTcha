using Microsoft.UI;
using Microsoft.UI.Input;
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
    private static readonly SolidColorBrush FrameStroke = new(Colors.Lime);
    private static readonly SolidColorBrush HandleFill = new(Windows.UI.Color.FromArgb(0xFF, 255, 255, 255));
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
        int left, top, right, bottom;

        if (fixedLeft)
            left = _dragStartX;
        else
            left = _dragStartX + _dragStartSize; // will be recalculated

        if (fixedTop)
            top = _dragStartY;
        else
            top = _dragStartY + _dragStartSize;

        if (fixedRight)
            right = _dragStartX + _dragStartSize;
        else
            right = _dragStartX + (int)Math.Round(dx);

        if (fixedBottom)
            bottom = _dragStartY + _dragStartSize;
        else
            bottom = _dragStartY + (int)Math.Round(dy);

        // from the corner constraints, compute square
        if (fixedLeft && fixedTop) // SE
        {
            var s = Math.Max(right - left, bottom - top);
            s = Clamp(s, 1, Math.Min(_imgW - left, _imgH - top));
            _cropSize = s;
            _cropX = left;
            _cropY = top;
        }
        else if (fixedLeft && fixedBottom) // NE
        {
            var s = Math.Max(right - left, _dragStartY + _dragStartSize - (top));
            var newTop = Clamp(_dragStartY + _dragStartSize - s, 0, _imgH - 1);
            s = Clamp(s, 1, Math.Min(_imgW - left, _imgH - newTop));
            _cropSize = s;
            _cropX = left;
            _cropY = _dragStartY + _dragStartSize - s;
        }
        else if (fixedRight && fixedTop) // SW
        {
            var s = Math.Max(right - (_dragStartX - _dragStartSize + (int)Math.Round(dx)), bottom - top);
            // simplification: use dx from west side
            var newLeft = _dragStartX + (int)Math.Round(dx);
            s = Math.Max(_dragStartX + _dragStartSize - newLeft, bottom - top);
            s = Clamp(s, 1, Math.Min(_imgW - newLeft, _imgH - top));
            _cropSize = s;
            _cropX = _dragStartX + _dragStartSize - s;
            _cropY = top;
        }
        else // NW — fixedRight && fixedBottom
        {
            var newLeft = _dragStartX + (int)Math.Round(dx);
            var newTop = _dragStartY + (int)Math.Round(dy);
            var s = Math.Max(_dragStartX + _dragStartSize - newLeft, _dragStartY + _dragStartSize - newTop);
            s = Clamp(s, 1, Math.Min(_imgW, _imgH));
            _cropSize = s;
            _cropX = _dragStartX + _dragStartSize - s;
            _cropY = _dragStartY + _dragStartSize - s;
        }

        // Clamp to image bounds
        _cropX = Clamp(_cropX, 0, _imgW - _cropSize);
        _cropY = Clamp(_cropY, 0, _imgH - _cropSize);
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
        OverlayCanvas.Children.Clear();
        if (_imgW <= 0 || _imgH <= 0 || _scale <= 0) return;

        var cw = OverlayCanvas.ActualWidth;
        var ch = OverlayCanvas.ActualHeight;
        var left = _ox + _cropX * _scale;
        var top = _oy + _cropY * _scale;
        var w = _cropSize * _scale;
        var h = _cropSize * _scale;

        // masks
        if (top > 0) AddMask(0, 0, cw, top);
        if (top + h < ch) AddMask(0, top + h, cw, ch - top - h);
        if (left > 0) AddMask(0, top, left, h);
        if (left + w < cw) AddMask(left + w, top, cw - left - w, h);

        // frame
        var rect = new Rectangle { Stroke = FrameStroke, StrokeThickness = 2, Width = w, Height = h };
        Canvas.SetLeft(rect, left);
        Canvas.SetTop(rect, top);
        OverlayCanvas.Children.Add(rect);

        // corner handles
        const double handleSz = 8;
        AddHandle(left - handleSz / 2, top - handleSz / 2, handleSz);
        AddHandle(left + w - handleSz / 2, top - handleSz / 2, handleSz);
        AddHandle(left - handleSz / 2, top + h - handleSz / 2, handleSz);
        AddHandle(left + w - handleSz / 2, top + h - handleSz / 2, handleSz);
    }

    private void AddHandle(double x, double y, double sz)
    {
        var h = new Rectangle
        {
            Fill = HandleFill,
            Stroke = FrameStroke,
            StrokeThickness = 1,
            Width = sz,
            Height = sz,
        };
        Canvas.SetLeft(h, x);
        Canvas.SetTop(h, y);
        OverlayCanvas.Children.Add(h);
    }

    private void AddMask(double x, double y, double w, double h)
    {
        if (w <= 0 || h <= 0) return;
        var r = new Rectangle { Fill = MaskBrush, Width = w, Height = h };
        Canvas.SetLeft(r, x);
        Canvas.SetTop(r, y);
        OverlayCanvas.Children.Add(r);
    }

    private static int Clamp(int v, int min, int max) => v < min ? min : v > max ? max : v;

    private static TaskCompletionSource<string?> CreateCompletionSource()
        => new(TaskCreationOptions.RunContinuationsAsynchronously);
}
