<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Title</title>
    <link rel="stylesheet" href="{{.CSS}}">
    <script src="{{.JS}}" defer></script>
    <script>
    (() => {
        const socketUrl = 'ws://'+window.location.host+'/ws';
        let socket = new WebSocket(socketUrl);
        socket.addEventListener('message', (evt) => {
            let data = JSON.parse(evt.data)
            window.location.href = data.url;
        });
    })();
    </script>
</head>
<body>
    {{.Body}}
</body>
</html>
