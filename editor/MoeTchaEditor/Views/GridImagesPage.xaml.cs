using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using MoeTchaEditor.Models;
using MoeTchaEditor.ViewModels;

namespace MoeTchaEditor.Views;

public sealed partial class GridImagesPage : Page
{
    private EditorViewModel VM => (EditorViewModel)DataContext;
    private bool _cropping;

    public GridImagesPage()
    {
        InitializeComponent();
        Unloaded += (_, _) =>
        {
            if (_cropping) CropOverlay.Cancel();
            _cropping = false;
        };
    }

    private async void ImportClick(object sender, RoutedEventArgs e)
    {
        if (_cropping || !VM.ImportGridImageCommand.CanExecute(null)) return;

        var existing = new HashSet<GridImageInfo>(
            VM.Pack.GridImages.Where(image => image != null)!);
        await VM.ImportGridImageCommand.ExecuteAsync(null);

        var imported = VM.Pack.GridImages
            .Where(image => image != null && !existing.Contains(image))
            .ToList();
        if (imported.Count == 0) return;

        var croppedCount = 0;
        for (var i = 0; i < imported.Count; i++)
        {
            var source = imported[i];
            var display = VM.GridImageDisplays.FirstOrDefault(item => ReferenceEquals(item.Source, source));
            if (display == null) continue;

            var cropped = await CropImageAsync(
                display,
                $"正在裁剪新导入图片 {i + 1}/{imported.Count}：{display.FileName}");
            if (cropped) croppedCount++;
        }

        VM.Status = croppedCount == imported.Count
            ? $"已导入并裁剪 {imported.Count} 张 Grid 图片"
            : $"已导入 {imported.Count} 张 Grid 图片，其中 {croppedCount} 张已裁剪";
    }

    private async void CropClick(object sender, RoutedEventArgs e)
    {
        if (sender is Button { Tag: GridImageDisplay display })
            await CropImageAsync(display, $"正在重新取景：{display.FileName}");
    }

    private async Task<bool> CropImageAsync(GridImageDisplay display, string status)
    {
        if (_cropping || VM.IsBusy) return false;

        _cropping = true;
        VM.IsBusy = true;
        try
        {
            VM.Status = status;
            var sourcePath = VM.EnsureGridCropSource(display);
            CropOverlay.Visibility = Visibility.Visible;
            var outputName = VM.GetCropOutputFileName(display);
            CropOverlay.Begin(sourcePath, VM.Pack.PackDirectory!, outputName);
            var newName = await CropOverlay.Completion;

            if (newName != null)
            {
                VM.ReplaceGridImageFile(display, newName);
                VM.Status = $"已裁剪：{newName}";
                return true;
            }

            VM.Status = "已跳过裁剪";
            return false;
        }
        catch (Exception ex)
        {
            VM.Status = $"裁剪失败：{ex.Message}";
            return false;
        }
        finally
        {
            VM.IsBusy = false;
            _cropping = false;
        }
    }

    private void DeleteClick(object sender, RoutedEventArgs e)
    {
        if (sender is Button { Tag: GridImageDisplay d })
            VM.DeleteGridImageCommand.Execute(d);
    }

    private void TagBoxLoaded(object sender, RoutedEventArgs e)
    {
        if (sender is AutoSuggestBox box)
            box.ItemsSource = VM.TagKeys;
    }

    private void TagBoxTextChanged(AutoSuggestBox sender, AutoSuggestBoxTextChangedEventArgs args)
    {
        if (args.Reason != AutoSuggestionBoxTextChangeReason.UserInput) return;

        var query = sender.Text;
        var lastComma = query.LastIndexOf(',');
        var currentWord = lastComma >= 0
            ? query[(lastComma + 1)..].TrimStart()
            : query.TrimStart();

        sender.ItemsSource = string.IsNullOrWhiteSpace(currentWord)
            ? VM.TagKeys
            : VM.TagKeys
                .Where(k => k.Contains(currentWord, StringComparison.OrdinalIgnoreCase))
                .ToList();
    }
}
