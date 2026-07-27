using Microsoft.UI.Dispatching;
using Microsoft.UI.Xaml;
using Microsoft.Windows.ApplicationModel.DynamicDependency;

namespace MoeTchaEditor;

public static class Program
{
    [STAThread]
    static void Main(string[] args)
    {
        var log = Path.Combine(
            Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData),
            "MoeTchaEditor", "boot.log");
        Directory.CreateDirectory(Path.GetDirectoryName(log)!);
        try
        {
            WinRT.ComWrappersSupport.InitializeComWrappers();
            File.AppendAllText(log, $"[{DateTime.Now}] 1-ComWrappers OK\n");

            Bootstrap.Initialize(0x00010008);
            File.AppendAllText(log, $"[{DateTime.Now}] 2-Bootstrap OK\n");

            File.AppendAllText(log, $"[{DateTime.Now}] 3-Calling Application.Start...\n");
            Application.Start((p) =>
            {
                File.AppendAllText(log, $"[{DateTime.Now}] 4-Start callback entered\n");
                var context = new DispatcherQueueSynchronizationContext(
                    DispatcherQueue.GetForCurrentThread());
                System.Threading.SynchronizationContext.SetSynchronizationContext(context);
                File.AppendAllText(log, $"[{DateTime.Now}] 5-Creating App...\n");
                new App();
                File.AppendAllText(log, $"[{DateTime.Now}] 6-App created\n");
            });
            File.AppendAllText(log, $"[{DateTime.Now}] 7-Application.Start returned\n");
        }
        catch (Exception ex)
        {
            File.AppendAllText(log, $"[{DateTime.Now}] CRASH: {ex}\n");
        }
    }
}
