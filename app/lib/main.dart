import 'dart:async';
import 'dart:convert';
import 'dart:developer';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:keep_screen_on/keep_screen_on.dart';

void main() {
  runApp(const MyApp());
}

class MyApp extends StatelessWidget {
  const MyApp({super.key});

  // This widget is the root of your application.
  @override
  Widget build(BuildContext context) {
    KeepScreenOn.turnOn();
    return MaterialApp(
      title: 'Bs Router',
      theme: ThemeData(
        // This is the theme of your application.
        //
        // TRY THIS: Try running your application with "flutter run". You'll see
        // the application has a blue toolbar. Then, without quitting the app,
        // try changing the seedColor in the colorScheme below to Colors.green
        // and then invoke "hot reload" (save your changes or press the "hot
        // reload" button in a Flutter-supported IDE, or press "r" if you used
        // the command line to start the app).
        //
        // Notice that the counter didn't reset back to zero; the application
        // state is not lost during the reload. To reset the state, use hot
        // restart instead.
        //
        // This works for code too, not just values: Most code changes can be
        // tested with just a hot reload.
        colorScheme: ColorScheme.fromSeed(seedColor: Colors.deepPurple),
        useMaterial3: true,
      ),
      home: const MyHomePage(title: 'Bs Router'),
    );
  }
}

class MyHomePage extends StatefulWidget {
  const MyHomePage({super.key, required this.title});

  // This widget is the home page of your application. It is stateful, meaning
  // that it has a State object (defined below) that contains fields that affect
  // how it looks.

  // This class is the configuration for the state. It holds the values (in this
  // case the title) provided by the parent (in this case the App widget) and
  // used by the build method of the State. Fields in a Widget subclass are
  // always marked "final".

  final String title;

  @override
  State<MyHomePage> createState() => _MyHomePageState();
}

class BsRouter {
  static const platform = MethodChannel('bsrouter');

  static Future<void> start() async {
    try {
      var res = await platform.invokeMethod("start");
      log("start service with $res");
    } catch (e) {
      log("start service error $e");
    }
  }

  static Future<void> stop() async {
    try {
      var res = await platform.invokeMethod("stop");
      log("stop service with $res");
    } catch (e) {
      log("stop service error $e");
    }
  }

  static Future<Map<String, dynamic>> state() async {
    var res = await platform.invokeMethod("state");
    return jsonDecode(res);
  }
}

class _MyHomePageState extends State<MyHomePage> {
  bool _run = false;
  String _title = "Router";
  String _text = "";
  Timer? _ticker;

  @override
  void initState() {
    super.initState();
    _refreshState();
    _startTick();
  }

  void _startTick() {
    _ticker ??= Timer.periodic(const Duration(seconds: 2), _onTick);
  }

  void _onTick(Timer timer) async {
    _refreshState();
  }

  void _refreshState() async {
    try {
      var text = "";
      var state = await BsRouter.state();
      var code = state["code"] as int? ?? -1;
      var name = state["name"] ?? "Router";
      if (code != 0) {
        _run = false;
        text += "State: not started\n";
        text += "Code: $code\n";
        text += "Message: ${state["message"]}\n";
        text += "Debug: ${state["debug"]}\n";
        if (code == 200) {
          await BsRouter.start();
        }
      } else {
        _run = true;
        text += "Channels:\n\n";
        (state["channels"] as Map<String, dynamic>).forEach((name, cs) {
          text += " >$name\n";
          for (Map<String, dynamic> channel in (cs as List<dynamic>)) {
            var last = DateTime.fromMillisecondsSinceEpoch(channel["ping"]["last"]);
            text += "  (${channel["id"]})";
            text += "  speed: ${channel["ping"]["speed"]} ms\n";
            text += "  last: $last\n";
            text += "  ${channel["connect"]}\n\n";
          }
          text += "\n";
        });

        text += "Table:\n";
        text += (state["table"] as List<dynamic>?)?.map((e) => " $e").join("\n") ?? "";
      }
      setState(() {
        _title = name;
        _text = text;
      });
    } catch (e, s) {
      log("refresh status error $e\n$s");
    }
  }

  void onControl() async {
    try {
      if (_run) {
        BsRouter.stop();
      } else {
        BsRouter.start();
      }
    } catch (e) {
      log("control error $e");
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        backgroundColor: Theme.of(context).colorScheme.inversePrimary,
        title: Text(_title),
      ),
      body: Padding(
        padding: const EdgeInsets.fromLTRB(16, 10, 0, 0),
        child: Column(
          mainAxisAlignment: MainAxisAlignment.start,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: <Widget>[Text(_text, textAlign: TextAlign.left)],
        ),
      ),
    );
  }

  @override
  void dispose() {
    super.dispose();
    _ticker?.cancel();
  }
}
