function InitApp() {

    var connection
    function get_ws2s() {
        let ws2s_input = $('#ws2s-url-input')
        window.ws2s_input = ws2s_input
        console.log("ws2s_input: ", ws2s_input)
        return ws2s_input.val()
    }
    function get_host() {
        return $('#host-input').val()
    }

    function get_port() {
        return parseInt($('#port-input').val())
    }

    window.config = {
        ws2s_host: get_ws2s(),
        host: get_host(),
        port: get_port()
    }
    console.log("remote conn configuration: ", window.config)

    $('#connect-button').bind("click", () => {
        connection = WS2S_connect(
            {
                ws2s_host: window.config.ws2s_host,
                host: window.config.host,
                port: window.config.port,
                on_connect: function() {
                    console.log('connected!'); },
                on_error: function(msg) { console.log('error: ' + msg); }
            }
        );
        console.log("connection ", connection);
    })

    $('#close-button').bind("click", () => {
        console.log("closing connection ", connection)
        WS2S_close({connection: connection, callback: () => {
            console.log("closed!")
        }})
    })

    function log(msg) {
        $('#log-textarea').val($("#log-textarea").val() + msg + "\n")
    }

    $('#send-button').bind("click",  () => {
        let data = $('#input-data-input').val()
        console.log("writing " + data + " to connection ", connection)
        WS2S_write({connection: connection, data: data, callback: (msg) => {
                console.log("was sent ? " + msg)
                $('#input-data-input').val("")
                log("> " + data)
            }
        })
    })

    $('#read-button').bind("click",  () => {
        WS2S_read({
            connection: connection,
            callback: function(input, err) {
                console.log("read err ?" + err)
                if (err === "") {
                    let str = String.fromCharCode.apply(null, input)
                    console.log("some recv data: " + str)
                    log("< " + str)
                }
            }
        })
    })
}