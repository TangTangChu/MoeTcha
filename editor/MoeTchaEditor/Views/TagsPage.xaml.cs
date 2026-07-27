using Microsoft.UI.Xaml.Controls;
using MoeTchaEditor.ViewModels;

namespace MoeTchaEditor.Views;

public sealed partial class TagsPage : Page
{
    private EditorViewModel VM => (EditorViewModel)DataContext;
    public TagsPage() => InitializeComponent();
}
