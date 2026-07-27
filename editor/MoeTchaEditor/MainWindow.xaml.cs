using Microsoft.UI.Xaml;
using Microsoft.UI.Windowing;
using MoeTchaEditor.ViewModels;
using Windows.Graphics;

namespace MoeTchaEditor;

public sealed partial class MainWindow : Window
{
    private bool _closingConfirmed;
    private bool _closeConfirmationOpen;

    public MainWindow()
    {
        InitializeComponent();

        ExtendsContentIntoTitleBar = true;
        SetTitleBar(AppTitleBar);

        AppWindow.SetIcon("Assets/AppIcon.ico");
        AppWindow.Resize(new SizeInt32(1200, 820));
        AppWindow.Closing += OnAppWindowClosing;

        RootFrame.Navigate(typeof(MainPage));
        if (RootFrame.Content is MainPage page)
        {
            page.VM.PropertyChanged += (_, args) =>
            {
                if (args.PropertyName == nameof(EditorViewModel.Title)) UpdateTitle(page.VM);
            };
            UpdateTitle(page.VM);
        }
    }

    private void OnAppWindowClosing(AppWindow sender, AppWindowClosingEventArgs args)
    {
        if (_closingConfirmed || RootFrame.Content is not MainPage page) return;
        if (page.VM.IsBusy)
        {
            args.Cancel = true;
            page.VM.Status = "请先完成当前操作再关闭";
            return;
        }
        if (!page.VM.IsDirty) return;

        args.Cancel = true;
        if (_closeConfirmationOpen) return;
        _closeConfirmationOpen = true;
        _ = ConfirmCloseAsync(page);
    }

    private async Task ConfirmCloseAsync(MainPage page)
    {
        try
        {
            if (!await page.ConfirmDiscardAsync()) return;
            _closingConfirmed = true;
            Close();
        }
        finally
        {
            _closeConfirmationOpen = false;
        }
    }

    private void UpdateTitle(EditorViewModel vm)
    {
        Title = vm.Title;
        AppTitleBar.Title = vm.Title;
    }
}
