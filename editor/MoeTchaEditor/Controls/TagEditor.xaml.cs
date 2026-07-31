using System.Collections.Generic;
using System.ComponentModel;
using System.Linq;
using Microsoft.UI;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Input;
using Microsoft.UI.Xaml.Media;
using Windows.Foundation;
using Windows.System;
using MoeTchaEditor.ViewModels;

namespace MoeTchaEditor.Controls;

/// <summary>
/// WinUI3 风格 token 式标签编辑器：chips 与输入框同处一个输入容器、流式换行；
/// 建议列表使用自绘 Popup（随滚动重定位），规避 AutoSuggestBox 在
/// ItemsRepeater 虚拟化 + ScrollViewer 场景下弹层丢失/无法弹出的问题。
/// </summary>
public sealed partial class TagEditor : UserControl
{
    /// <summary>建议浮层中的单条候选（Key + 引用数元信息）。</summary>
    public sealed class SuggestionItem
    {
        public string Key { get; }
        public string Meta { get; }
        public bool HasMeta => Meta.Length > 0;

        public SuggestionItem(string key, string meta)
        {
            Key = key;
            Meta = meta;
        }
    }

    public static readonly DependencyProperty TagsProperty =
        DependencyProperty.Register(nameof(Tags), typeof(IList<string>), typeof(TagEditor),
            new PropertyMetadata(null, OnTagsChanged));

    /// <summary>整体替换为新 <see cref="List{T}"/> 以触发源对象重建索引。</summary>
    public IList<string>? Tags
    {
        get => (IList<string>?)GetValue(TagsProperty);
        set => SetValue(TagsProperty, value);
    }

    public static readonly DependencyProperty SuggestionsProperty =
        DependencyProperty.Register(nameof(Suggestions), typeof(IReadOnlyList<string>), typeof(TagEditor),
            new PropertyMetadata(null, OnSuggestionsChanged));

    public IReadOnlyList<string>? Suggestions
    {
        get => (IReadOnlyList<string>?)GetValue(SuggestionsProperty);
        set => SetValue(SuggestionsProperty, value);
    }

    public static readonly DependencyProperty MaxTagsProperty =
        DependencyProperty.Register(nameof(MaxTags), typeof(int), typeof(TagEditor),
            new PropertyMetadata(0));

    /// <summary>最大标签数；0 表示不限。设为 1 即单选模式：新增标签会替换已有标签。</summary>
    public int MaxTags
    {
        get => (int)GetValue(MaxTagsProperty);
        set => SetValue(MaxTagsProperty, value);
    }

    public static readonly DependencyProperty PlaceholderTextProperty =
        DependencyProperty.Register(nameof(PlaceholderText), typeof(string), typeof(TagEditor),
            new PropertyMetadata("添加标签…"));

    /// <summary>输入框占位提示，默认“添加标签…”。</summary>
    public string PlaceholderText
    {
        get => (string)GetValue(PlaceholderTextProperty);
        set => SetValue(PlaceholderTextProperty, value);
    }

    private readonly List<string> _rendered = new();
    private EditorViewModel? _vm;
    private bool _isPointerOver;
    private bool _isFocused;
    private bool _suppressTextChanged;
    private Point _lastPopupOffset = new(double.NaN, double.NaN);
    private double _lastPopupWidth = double.NaN;

    public TagEditor()
    {
        InitializeComponent();
        Loaded += OnRootLoaded;
        Unloaded += OnRootUnloaded;
        LayoutUpdated += OnLayoutUpdated;
    }

    // ─────────────── 数据源 ───────────────

    private static void OnTagsChanged(DependencyObject d, DependencyPropertyChangedEventArgs e)
        => ((TagEditor)d).RebuildChips();

    private static void OnSuggestionsChanged(DependencyObject d, DependencyPropertyChangedEventArgs e)
        => ((TagEditor)d).OnSuggestionsSourceChanged();

    // 从可视树上层的 EditorViewModel 取 TagKeys；ElementName 绑定在 ItemsRepeater 的 DataTemplate 里不可靠。
    private void OnRootLoaded(object sender, RoutedEventArgs e)
    {
        DetachVm();
        var fe = VisualTreeHelper.GetParent(this) as FrameworkElement;
        while (fe != null)
        {
            if (fe.DataContext is EditorViewModel vm)
            {
                _vm = vm;
                break;
            }
            fe = VisualTreeHelper.GetParent(fe) as FrameworkElement;
        }

        if (_vm != null)
        {
            _vm.PropertyChanged += OnVmPropertyChanged;
            Suggestions = _vm.TagKeys;
        }
    }

    private void OnRootUnloaded(object sender, RoutedEventArgs e)
    {
        CloseSuggestions();
        DetachVm();
    }

    private void DetachVm()
    {
        if (_vm != null)
            _vm.PropertyChanged -= OnVmPropertyChanged;
        _vm = null;
    }

    private void OnVmPropertyChanged(object? sender, PropertyChangedEventArgs e)
    {
        if (e.PropertyName == nameof(EditorViewModel.TagKeys))
            Suggestions = _vm?.TagKeys;
    }

    private void OnSuggestionsSourceChanged()
    {
        // 建议源整体刷新（如其他卡片改了标签）时，同步已打开的浮层
        if (SuggestionPopup.IsOpen)
            RefreshSuggestionItems();
    }

    // ─────────────── chips：增量 diff，避免整表重建丢焦点 / 重复分配 ───────────────
    // 约定：ChipPanel.Children[0.._rendered.Count) 是 chip，其后依次是输入框。

    private void RebuildChips()
    {
        var tags = Tags ?? (IList<string>)Array.Empty<string>();
        if (tags.Count == _rendered.Count && tags.SequenceEqual(_rendered))
            return; // 未变跳过，避免丢焦点

        int common = 0;
        while (common < _rendered.Count && common < tags.Count && _rendered[common] == tags[common])
            common++;

        // 尾删：只移除变化的区间，输入框始终保留（焦点、IME 上下文不丢）
        for (int i = _rendered.Count - 1; i >= common; i--)
            ChipPanel.Children.RemoveAt(i);

        for (int i = common; i < tags.Count; i++)
            ChipPanel.Children.Insert(i, MakeChip(tags[i]));

        _rendered.Clear();
        _rendered.AddRange(tags);
    }

    private FrameworkElement MakeChip(string tag)
    {
        var close = new Button
        {
            Content = new FontIcon { Glyph = "\uE711", FontSize = 10 },
            Padding = new Thickness(3, 0, 3, 0),
            MinWidth = 0,
            MinHeight = 0,
            Background = null,
            BorderThickness = new Thickness(0),
            Foreground = ResolveBrush("TextFillColorSecondaryBrush") ?? new SolidColorBrush(Colors.Gray),
            VerticalAlignment = VerticalAlignment.Center,
            Tag = tag,
        };
        close.Click += OnChipClose;

        var text = new TextBlock
        {
            Text = tag,
            FontSize = 12,
            VerticalAlignment = VerticalAlignment.Center,
            TextTrimming = TextTrimming.CharacterEllipsis,
            MaxWidth = 160,
        };
        ToolTipService.SetToolTip(text, tag);

        var stack = new StackPanel
        {
            Orientation = Orientation.Horizontal,
            Spacing = 2,
            VerticalAlignment = VerticalAlignment.Center,
        };
        stack.Children.Add(text);
        stack.Children.Add(close);

        // Fluent chip：subtle 底 + 悬停加深，无描边（容器描边统一负责边界）
        var rest = ResolveBrush("SubtleFillColorSecondaryBrush") ?? new SolidColorBrush(Colors.Transparent);
        var hover = ResolveBrush("SubtleFillColorTertiaryBrush") ?? rest;

        var chip = new Border
        {
            Background = rest,
            CornerRadius = new CornerRadius(6),
            Padding = new Thickness(8, 3, 4, 3),
            Child = stack,
            VerticalAlignment = VerticalAlignment.Center,
            Tag = tag,
        };
        chip.PointerEntered += (_, _) =>
        {
            chip.Background = hover;
            close.Foreground = ResolveBrush("TextFillColorPrimaryBrush") ?? close.Foreground;
        };
        chip.PointerExited += (_, _) =>
        {
            chip.Background = rest;
            close.Foreground = ResolveBrush("TextFillColorSecondaryBrush") ?? close.Foreground;
        };
        return chip;
    }

    private void OnChipClose(object sender, RoutedEventArgs e)
    {
        if (sender is Button b && b.Tag is string tag)
            RemoveTag(tag);
    }

    // ─────────────── 增删 ───────────────

    private void AddTag(string? raw)
    {
        var tag = raw?.Trim() ?? "";
        if (tag.Length == 0) return;

        var current = Tags != null ? new List<string>(Tags) : new List<string>();
        if (current.Contains(tag)) return; // 去重，与模型 Distinct(Ordinal) 一致

        // MaxTags==1：单选模式，新标签替换已有标签（用于 Click 区域单标签等场景）
        if (MaxTags == 1 && current.Count > 0)
            current.Clear();

        current.Add(tag);
        Tags = current;
    }

    private void RemoveTag(string tag)
    {
        if (Tags == null) return;
        var current = new List<string>(Tags);
        if (!current.Remove(tag)) return;

        Tags = current;
        _ = Input.Focus(FocusState.Programmatic);
    }

    // ─────────────── 建议浮层 ───────────────

    private void OnInputGotFocus(object sender, RoutedEventArgs e)
    {
        _isFocused = true;
        UpdateRootVisual();
        OpenSuggestions();
    }

    private void OnInputLostFocus(object sender, RoutedEventArgs e)
    {
        _isFocused = false;
        UpdateRootVisual();
        // 这里【不】关闭浮层：light-dismiss 已负责外部点击关闭；
        // 若在此同步 CloseSuggestions，Popup 关闭把焦点还给输入框会再次 GotFocus，
        // 与 Popup 打开抢焦点形成同步递归，最终栈溢出（STATUS_STACK_BUFFER_OVERRUN）。
    }

    private void OnInputTextChanged(object sender, TextChangedEventArgs e)
    {
        if (_suppressTextChanged) return;
        if (SuggestionPopup.IsOpen)
            RefreshSuggestionItems();
        else
            OpenSuggestions();
    }

    private void OpenSuggestions()
    {
        if (SuggestionPopup.IsOpen)
        {
            // 已打开则仅重定位，防重入（IsOpen=true 重复设置可能再次触发焦点转移）
            UpdatePopupPosition();
            return;
        }

        RefreshSuggestionItems();
        if (SuggestionPopup.XamlRoot == null && XamlRoot != null)
            SuggestionPopup.XamlRoot = XamlRoot;
        UpdatePopupPosition();
        SuggestionPopup.IsOpen = true;
        // Popup 打开可能抢走输入焦点，立即归还，避免 GotFocus/LostFocus 震荡
        _ = Input.Focus(FocusState.Programmatic);
    }

    private void CloseSuggestions()
    {
        if (SuggestionPopup.IsOpen)
            SuggestionPopup.IsOpen = false;
    }

    private void RefreshSuggestionItems()
    {
        var query = Input.Text?.Trim() ?? "";
        var src = Suggestions;
        var items = new List<SuggestionItem>(8);
        if (src != null && src.Count > 0)
        {
            IEnumerable<string> matched = query.Length == 0
                ? src
                : src.Where(k => k.Contains(query, StringComparison.OrdinalIgnoreCase));
            foreach (var key in matched) // MaxHeight 已限高并触发内部滚动，无需再截断
                items.Add(new SuggestionItem(key, Describe(key)));
        }

        SuggestionList.ItemsSource = items;
        if (SuggestionList.SelectedIndex >= items.Count)
            SuggestionList.SelectedIndex = -1;

        NoMatchHint.Text = query.Length == 0 ? "暂无已定义标签" : "无匹配标签，按 Enter 新建";
        NoMatchHint.Visibility = items.Count == 0 ? Visibility.Visible : Visibility.Collapsed;
    }

    private string Describe(string key)
    {
        var usage = _vm?.TagIndex.Get(key);
        if (usage == null) return "";
        var parts = new List<string>(3);
        if (usage.GridCount > 0) parts.Add($"{usage.GridCount} 图");
        if (usage.ClickCount > 0) parts.Add($"{usage.ClickCount} 区域");
        if (usage.SimilarCount > 0) parts.Add($"{usage.SimilarCount} 相似");
        return parts.Count > 0 ? string.Join(" · ", parts) : "未使用";
    }

    private void OnSuggestionItemClick(object sender, ItemClickEventArgs e)
    {
        if (e.ClickedItem is SuggestionItem item)
            PickSuggestion(item);
    }

    private void PickSuggestion(SuggestionItem item)
    {
        AddTag(item.Key);
        SetInputText("");
        CloseSuggestions();
        _ = Input.Focus(FocusState.Programmatic);
    }

    private void CommitTyped()
    {
        AddTag(Input.Text);
        SetInputText("");
        CloseSuggestions();
        _ = Input.Focus(FocusState.Programmatic);
    }

    private void SetInputText(string text)
    {
        _suppressTextChanged = true;
        try
        {
            Input.Text = text;
        }
        finally
        {
            _suppressTextChanged = false;
        }
    }

    private void MoveSelection(int delta)
    {
        var count = SuggestionList.Items.Count;
        if (count == 0) return;
        var index = SuggestionList.SelectedIndex;
        var next = index < 0
            ? (delta > 0 ? 0 : count - 1)
            : Math.Clamp(index + delta, 0, count - 1);
        SuggestionList.SelectedIndex = next;
        SuggestionList.ScrollIntoView(SuggestionList.SelectedItem);
    }

    // ─────────────── 键盘 ───────────────

    private void OnInputKeyDown(object sender, KeyRoutedEventArgs e)
    {
        switch (e.Key)
        {
            case VirtualKey.Enter:
                if (SuggestionPopup.IsOpen
                    && SuggestionList.SelectedIndex >= 0
                    && SuggestionList.SelectedItem is SuggestionItem picked)
                {
                    PickSuggestion(picked);
                }
                else
                {
                    CommitTyped();
                }
                e.Handled = true;
                break;

            case VirtualKey.Down:
                if (!SuggestionPopup.IsOpen)
                    OpenSuggestions();
                MoveSelection(1);
                e.Handled = true;
                break;

            case VirtualKey.Up:
                if (SuggestionPopup.IsOpen)
                {
                    MoveSelection(-1);
                    e.Handled = true;
                }
                break;

            case VirtualKey.Escape:
                if (SuggestionPopup.IsOpen)
                {
                    CloseSuggestions();
                }
                else
                {
                    SetInputText("");
                    RefreshSuggestionItems();
                }
                e.Handled = true;
                break;

            case VirtualKey.Back when string.IsNullOrEmpty(Input.Text):
                if (_rendered.Count > 0)
                    RemoveTag(_rendered[^1]);
                e.Handled = true;
                break;
        }
    }

    // ─────────────── 容器视觉状态（Fluent 输入框：rest / hover / focused）───────────────

    private void OnRootPointerEntered(object sender, PointerRoutedEventArgs e)
    {
        _isPointerOver = true;
        UpdateRootVisual();
    }

    private void OnRootPointerExited(object sender, PointerRoutedEventArgs e)
    {
        _isPointerOver = false;
        UpdateRootVisual();
    }

    private void OnRootPointerPressed(object sender, PointerRoutedEventArgs e)
    {
        // 点容器空白/chips 处也聚焦输入框；点输入框内部（OriginalSource 是其内部元素）不干预
        if (e.OriginalSource is DependencyObject src && !IsDescendantOf(src, Input))
            _ = Input.Focus(FocusState.Programmatic);
    }

    private void UpdateRootVisual()
    {
        RootBorder.Background = _isFocused
            ? ResolveBrush("ControlFillColorInputActiveBrush")
              ?? ResolveBrush("ControlFillColorDefaultBrush")
            : _isPointerOver
                ? ResolveBrush("ControlFillColorSecondaryBrush")
                  ?? ResolveBrush("ControlFillColorDefaultBrush")
                : ResolveBrush("ControlFillColorDefaultBrush");
        RootBorder.BorderBrush = _isFocused
            ? ResolveBrush("AccentFillColorDefaultBrush")
            : ResolveBrush("ControlStrokeColorDefaultBrush");
    }

    // ─────────────── 浮层定位：滚动 / 尺寸变化时随控件移动 ───────────────

    private void OnLayoutUpdated(object? sender, object e)
    {
        if (SuggestionPopup.IsOpen)
            UpdatePopupPosition();
    }

    private void UpdatePopupPosition()
    {
        if (XamlRoot == null) return;

        // Popup 的 HorizontalOffset/VerticalOffset 是相对其 XAML 父级（本控件根 Grid）的偏移，
        // 而非相对窗口根。用 null 会得到窗口根坐标，叠加父级位置后弹层会被推到窗口底部。
        var parent = SuggestionPopup.Parent as UIElement ?? this;
        var anchor = RootBorder.TransformToVisual(parent)
            .TransformPoint(new Point(0, RootBorder.ActualHeight + 4));

        var w = RootBorder.ActualWidth;
        if (double.IsNaN(w)) w = 180;
        var width = Math.Clamp(w, 180, 360);

        if (anchor.Equals(_lastPopupOffset) && width.Equals(_lastPopupWidth))
            return;

        _lastPopupOffset = anchor;
        _lastPopupWidth = width;
        SuggestionSurface.Width = width;
        SuggestionPopup.HorizontalOffset = anchor.X;
        SuggestionPopup.VerticalOffset = anchor.Y;
    }

    private static bool IsDescendantOf(DependencyObject? node, DependencyObject? ancestor)
    {
        while (node != null && !ReferenceEquals(node, ancestor))
            node = VisualTreeHelper.GetParent(node);
        return node != null;
    }

    private static Brush? ResolveBrush(string key)
        => Application.Current.Resources.TryGetValue(key, out var b) && b is Brush brush ? brush : null;
}
