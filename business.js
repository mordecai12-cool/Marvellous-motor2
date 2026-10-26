(() => {
 document.querySelectorAll('[data-business-filter]').forEach(input=>input.addEventListener('input',()=>{const query=input.value.toLowerCase();document.querySelectorAll('[data-business-row]').forEach(row=>row.hidden=!row.textContent.toLowerCase().includes(query));}));
})();
