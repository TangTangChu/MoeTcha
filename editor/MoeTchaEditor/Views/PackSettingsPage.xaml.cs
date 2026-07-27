using Microsoft.UI.Xaml.Controls;
using MoeTchaEditor.ViewModels;

namespace MoeTchaEditor.Views;

public sealed partial class PackSettingsPage : Page
{
    private EditorViewModel VM => (EditorViewModel)DataContext;
    private bool _updatingGridConfig;

    public PackSettingsPage() => InitializeComponent();

    private void GridSizeChanged(NumberBox sender, NumberBoxValueChangedEventArgs args)
    {
        if (_updatingGridConfig || double.IsNaN(args.NewValue)) return;
        _updatingGridConfig = true;
        try
        {
            var grid = VM.Pack.Grid;
            var size = Math.Clamp((int)Math.Round(args.NewValue), 4, 36);
            grid.Size = size;
            if (grid.CorrectMax >= size) grid.CorrectMax = size - 1;
            if (grid.CorrectMin > grid.CorrectMax) grid.CorrectMin = grid.CorrectMax;
        }
        finally
        {
            _updatingGridConfig = false;
        }
    }

    private void CorrectMinChanged(NumberBox sender, NumberBoxValueChangedEventArgs args)
    {
        if (_updatingGridConfig || double.IsNaN(args.NewValue)) return;
        _updatingGridConfig = true;
        try
        {
            var grid = VM.Pack.Grid;
            var min = Math.Clamp((int)Math.Round(args.NewValue), 1, Math.Max(1, grid.Size - 1));
            grid.CorrectMin = min;
            if (grid.CorrectMax < min) grid.CorrectMax = min;
        }
        finally
        {
            _updatingGridConfig = false;
        }
    }

    private void CorrectMaxChanged(NumberBox sender, NumberBoxValueChangedEventArgs args)
    {
        if (_updatingGridConfig || double.IsNaN(args.NewValue)) return;
        _updatingGridConfig = true;
        try
        {
            var grid = VM.Pack.Grid;
            var max = Math.Clamp((int)Math.Round(args.NewValue), 1, Math.Max(1, grid.Size - 1));
            grid.CorrectMax = max;
            if (grid.CorrectMin > max) grid.CorrectMin = max;
        }
        finally
        {
            _updatingGridConfig = false;
        }
    }
}
