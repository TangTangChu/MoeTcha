using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Windows.Foundation;

namespace MoeTchaEditor.Controls;

/// <summary>
/// 逐项换行的流式布局：宽度不够时自动折到下一行，行内子元素垂直居中。
/// 通过 <see cref="Spacing"/> 控制水平/垂直间距；
/// 子元素可附加 <see cref="SetIsFill"/> 让其填满本行剩余宽度（用于内嵌输入框，自动响应容器宽度）。
/// </summary>
public sealed class WrapPanel : Panel
{
    public static readonly DependencyProperty SpacingProperty =
        DependencyProperty.Register(nameof(Spacing), typeof(double), typeof(WrapPanel),
            new PropertyMetadata(0.0, OnLayoutPropertyChanged));

    /// <summary>子元素之间及行与行之间的间距。</summary>
    public double Spacing
    {
        get => (double)GetValue(SpacingProperty);
        set => SetValue(SpacingProperty, value);
    }

    /// <summary>附加属性：设为 true 的子元素会拉伸填满本行剩余宽度。通常是编辑器里的输入框。</summary>
    public static readonly DependencyProperty IsFillProperty =
        DependencyProperty.RegisterAttached("IsFill", typeof(bool), typeof(WrapPanel),
            new PropertyMetadata(false, OnLayoutPropertyChanged));

    public static bool GetIsFill(DependencyObject obj) => (bool)obj.GetValue(IsFillProperty);
    public static void SetIsFill(DependencyObject obj, bool value) => obj.SetValue(IsFillProperty, value);

    private static void OnLayoutPropertyChanged(DependencyObject d, DependencyPropertyChangedEventArgs e)
        => (d as WrapPanel)?.InvalidateMeasure();

    protected override Size MeasureOverride(Size availableSize)
    {
        var spacing = Spacing;
        // 测量时给「填充」子一个受限宽度，避免它把自身撑到整行宽而干扰换行判定；
        // 排列阶段再按剩余宽度拉伸。
        foreach (var child in Children)
        {
            if (child == null) continue;
            var isFill = GetIsFill(child);
            var measureWidth = isFill && !double.IsInfinity(availableSize.Width)
                ? Math.Min(availableSize.Width, 240)
                : availableSize.Width;
            child.Measure(new Size(measureWidth, availableSize.Height));
        }

        var rows = BuildRows(availableSize.Width);
        double totalHeight = 0;
        foreach (var row in rows)
        {
            if (row.Count == 0) continue;
            totalHeight += row.Max(c => c.DesiredSize.Height);
        }
        totalHeight += Math.Max(0, rows.Count - 1) * spacing;

        var returnWidth = double.IsInfinity(availableSize.Width) ? 0 : availableSize.Width;
        return new Size(returnWidth, totalHeight);
    }

    protected override Size ArrangeOverride(Size finalSize)
    {
        var spacing = Spacing;
        var rows = BuildRows(finalSize.Width);

        double y = 0;
        foreach (var row in rows)
        {
            if (row.Count == 0) continue;
            double rowHeight = row.Max(c => c.DesiredSize.Height);
            double x = 0;
            foreach (var child in row)
            {
                var isFill = GetIsFill(child);
                var req = child.DesiredSize;

                var width = isFill
                    ? Math.Max(req.Width, finalSize.Width - x)
                    : req.Width;
                var yOffset = y + (rowHeight - req.Height) / 2;

                child.Arrange(new Rect(x, yOffset, Math.Max(0, width), req.Height));

                if (isFill)
                    x = finalSize.Width; // 填充子吞掉本行剩余宽度
                else
                    x += width + spacing;
            }
            y += rowHeight + spacing;
        }

        return new Size(finalSize.Width, rows.Count > 0 ? Math.Max(0, y - spacing) : 0);
    }

    /// <summary>按当前宽度把子元素分组成若干行。宽度为无穷大时不换行（单行）。</summary>
    private List<List<UIElement>> BuildRows(double width)
    {
        var spacing = Spacing;
        var wrap = !double.IsInfinity(width) && width > 0;
        var rows = new List<List<UIElement>>();
        var row = new List<UIElement>();
        double x = 0;

        foreach (var child in Children)
        {
            if (child == null) continue;
            var isFill = GetIsFill(child);
            var req = child.DesiredSize;

            // 需要宽度：fill 子可压缩到自身 MinWidth，其余用实际期望宽度；
            // fill 子同样参与换行判定，避免 chips 占满一行时把输入框压到 0 宽
            var need = req.Width;
            if (isFill && child is FrameworkElement fe && fe.MinWidth > need)
                need = fe.MinWidth;

            if (wrap && x > 0 && x + need > width)
            {
                rows.Add(row);
                row = new List<UIElement>();
                x = 0;
            }

            row.Add(child);

            if (isFill)
            {
                // 填充子总在行尾：吞掉剩余宽度后换行
                rows.Add(row);
                row = new List<UIElement>();
                x = 0;
            }
            else
            {
                x += req.Width + spacing;
            }
        }

        if (row.Count > 0) rows.Add(row);
        return rows;
    }
}
