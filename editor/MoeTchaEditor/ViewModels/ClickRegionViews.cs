using System;
using System.Collections.Generic;
using System.Collections.ObjectModel;
using System.ComponentModel;
using System.Linq;
using CommunityToolkit.Mvvm.ComponentModel;
using Microsoft.UI.Xaml.Media;
using Windows.UI;
using MoeTchaEditor.Models;

namespace MoeTchaEditor.ViewModels;

/// <summary>区域行视图：包装 <see cref="RegionInfo"/>，承载选中态、坐标文案与配色。</summary>
public sealed class RegionRow : ObservableObject
{
    public RegionInfo Source { get; }

    /// <summary>该区域所属标签的描边色（也用于句柄）。</summary>
    public SolidColorBrush Stroke { get; }

    /// <summary>半透明填充色。</summary>
    public SolidColorBrush Fill { get; }

    /// <summary>标签 chip 的底色。</summary>
    public SolidColorBrush ChipBrush { get; }

    private bool _isSelected;
    public bool IsSelected
    {
        get => _isSelected;
        set => SetProperty(ref _isSelected, value);
    }

    public string Coords => $"{Source.X},{Source.Y}  {Source.Width}×{Source.Height}";

    private IList<string> _tags;
    /// <summary>单标签适配为列表，供 TagEditor（MaxTags=1）双向绑定。</summary>
    public IList<string> Tags
    {
        get => _tags;
        set
        {
            var v = value ?? new List<string>();
            if (ReferenceEquals(_tags, v)) return;
            _tags = v;
            OnPropertyChanged(nameof(Tags));
            var t = v.Count > 0 ? v[0] : "";
            if (!string.Equals(Source.Tag, t, StringComparison.Ordinal))
                Source.Tag = t;
        }
    }

    public RegionRow(RegionInfo source, SolidColorBrush stroke, SolidColorBrush fill, SolidColorBrush chip)
    {
        Source = source;
        Stroke = stroke;
        Fill = fill;
        ChipBrush = chip;
        _tags = string.IsNullOrEmpty(source.Tag) ? new List<string>() : new List<string> { source.Tag };
        source.PropertyChanged += OnSourcePropertyChanged;
    }

    /// <summary>从 Source 解除事件订阅，避免 RebuildGroups 重建时事件泄漏。</summary>
    public void Detach() => Source.PropertyChanged -= OnSourcePropertyChanged;

    private void OnSourcePropertyChanged(object? sender, PropertyChangedEventArgs e)
    {
        if (e.PropertyName is nameof(RegionInfo.X) or nameof(RegionInfo.Y)
            or nameof(RegionInfo.Width) or nameof(RegionInfo.Height))
        {
            OnPropertyChanged(nameof(Coords));
        }
        else if (e.PropertyName == nameof(RegionInfo.Tag))
        {
            // 外部改动 Source.Tag 时同步（正常编辑路径由 Tags setter 先行更新，不会走到这里）
            var t = Source.Tag ?? "";
            var want = t.Length == 0 ? new List<string>() : new List<string> { t };
            if (!_tags.SequenceEqual(want))
            {
                _tags = want;
                OnPropertyChanged(nameof(Tags));
            }
        }
    }
}

/// <summary>标签组视图：按 tag 聚合区域，承载激活态与数量。</summary>
public sealed class RegionGroup : ObservableObject
{
    /// <summary>组对应的标签（只读，由区域内 tag 派生）。</summary>
    public string Tag { get; }

    /// <summary>组的主色。</summary>
    public SolidColorBrush Color { get; }

    /// <summary>是否为当前「绘制目标」标签：新建区域会自动归入此组。</summary>
    private bool _isActive;
    public bool IsActive
    {
        get => _isActive;
        set => SetProperty(ref _isActive, value);
    }

    public ObservableCollection<RegionRow> Rows { get; } = [];

    public int Count => Rows.Count;

    /// <summary>组标题：空标签显示为「未标记」。</summary>
    public string DisplayTag => string.IsNullOrEmpty(Tag) ? "未标记" : Tag;

    public RegionGroup(string tag, SolidColorBrush color)
    {
        Tag = tag ?? "";
        Color = color;
    }

    public void RefreshCount() => OnPropertyChanged(nameof(Count));
}
