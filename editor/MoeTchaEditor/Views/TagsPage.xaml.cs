using Microsoft.UI.Xaml.Controls;
using MoeTchaEditor.ViewModels;

namespace MoeTchaEditor.Views;

public sealed partial class TagsPage : Page
{
    private EditorViewModel VM => (EditorViewModel)DataContext;
    public TagsPage() => InitializeComponent();

    private void TagSearchTextChanged(AutoSuggestBox sender, AutoSuggestBoxTextChangedEventArgs args)
    {
        if (args.Reason != AutoSuggestionBoxTextChangeReason.UserInput) return;
        var query = sender.Text.Trim();
        var allKeys = VM.TagUsages.Select(t => t.Key).ToList();
        sender.ItemsSource = string.IsNullOrEmpty(query)
            ? allKeys
            : allKeys.Where(k => k.Contains(query, StringComparison.OrdinalIgnoreCase)).ToList();
    }
}
