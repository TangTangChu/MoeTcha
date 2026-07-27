using Microsoft.UI.Xaml.Controls;
using MoeTchaEditor.ViewModels;
using MoeTchaEditor.Views;

namespace MoeTchaEditor;

public sealed partial class MainPage : Page
{
    public EditorViewModel VM { get; } = new();
    private bool _discardConfirmationOpen;
    private readonly Dictionary<string, Page> _pages = [];

    public MainPage()
    {
        InitializeComponent();
        VM.ConfirmDiscardAsync = ConfirmDiscardAsync;
        ContentFrame.Content = GetPage("settings");

        VM.PropertyChanged += (_, e) =>
        {
            if (e.PropertyName == nameof(VM.HasPack) && VM.HasPack && MainNav.MenuItems.Count > 0)
            {
                // 打开/新建 pack 后自动选中第一个导航项，确保正确页面展示
                MainNav.SelectedItem = MainNav.MenuItems[0];
            }
        };

    }

    public async Task<bool> ConfirmDiscardAsync()
    {
        if (!VM.IsDirty) return true;
        if (XamlRoot == null || _discardConfirmationOpen) return false;

        _discardConfirmationOpen = true;
        try
        {
            var dialog = new ContentDialog
            {
                Title = "存在未保存修改",
                Content = "当前素材包有未保存的修改，要先保存吗？",
                PrimaryButtonText = "放弃修改",
                SecondaryButtonText = "保存",
                CloseButtonText = "取消",
                DefaultButton = ContentDialogButton.Secondary,
                XamlRoot = XamlRoot,
            };

            var result = await dialog.ShowAsync();
            if (result == ContentDialogResult.Primary) return true;
            if (result != ContentDialogResult.Secondary) return false;

            VM.SaveCommand.Execute(null);
            return !VM.IsDirty;
        }
        finally
        {
            _discardConfirmationOpen = false;
        }
    }

    private void NavSelectionChanged(NavigationView sender, NavigationViewSelectionChangedEventArgs args)
    {
        if (args.SelectedItem is not NavigationViewItem { Tag: string tag }) return;
        ContentFrame.Content = GetPage(tag);
    }

    private Page GetPage(string tag)
    {
        if (_pages.TryGetValue(tag, out var page)) return page;

        page = tag switch
        {
            "tags" => new TagsPage(),
            "grid" => new GridImagesPage(),
            "click" => new ClickImagesPage(),
            _ => new PackSettingsPage(),
        };
        page.DataContext = VM;
        _pages[tag] = page;
        return page;
    }
}
