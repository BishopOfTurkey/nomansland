
var activities = document.getElementsByClassName("activity")

var out = []
for (var i = 0; i < activities.length; i++) {
    a = {
        "name": activities[i].getElementsByClassName("entry-athlete")[0].text.trim(), // name
        "date": activities[i].getElementsByClassName("timestamp")[0].attributes['datetime'].textContent.trim(),
        "distance": activities[i].getElementsByClassName("inline-stats")[0].children[0].textContent.trim(),
        "duration": activities[i].getElementsByClassName("inline-stats")[0].children[2].textContent.trim(),
        "title": activities[i].getElementsByClassName("activity-title")[0].textContent.trim()
    }
    out.push(a)
}

function download(filename, text) {
    var pom = document.createElement('a');
    pom.setAttribute('href', 'data:text/plain;charset=utf-8,' + encodeURIComponent(text));
    pom.setAttribute('download', filename);

    if (document.createEvent) {
        var event = document.createEvent('MouseEvents');
        event.initEvent('click', true, true);
        pom.dispatchEvent(event);
    }
    else {
        pom.click();
    }
}

var a = JSON.stringify(out)
download("club_data.json", a)