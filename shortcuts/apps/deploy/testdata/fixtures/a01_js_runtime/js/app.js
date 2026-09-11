
fetch('./data.json');                       // → 文档相对 → /data.json
window.fetch('./NOT-collected.json');       // 成员调用，不收
const w = new Worker('./worker.js');        // → /worker.js
const u = new URL('./sibling.js', import.meta.url);  // 原样 → js/sibling.js
const xhr = new XMLHttpRequest();
xhr.open('GET', './xhr.json');              // → /xhr.json
navigator.serviceWorker.register('./sw.js');// → /sw.js
loadJSON(`./tpl.json`);                     // 模板串 → /tpl.json
fetch(someVar);                             // 非字面量 → unsupported
