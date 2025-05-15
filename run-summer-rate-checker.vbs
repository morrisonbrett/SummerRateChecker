' run-summer-rate-checker.vbs - Use Task Scheduler "Start in" directory
Set WshShell = CreateObject("WScript.Shell")

' Set working directory to current directory
WshShell.CurrentDirectory = "."

' Run with relative path
WshShell.Run "bin\SummerRateChecker.exe", 0, False