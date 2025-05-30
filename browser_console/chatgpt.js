// Auto-continue script for chat interface
// This script will type "CONTINUE" and submit every 2 minutes until "End of script" is found in DOM

let autoSubmitInterval;

function checkForEndOfScript() {
    // Check if "End of script" text exists more than 1 time anywhere in the DOM
    const bodyText = document.body.innerText || document.body.textContent || '';
    const matches = (bodyText.match(/End of script/g) || []).length;
    console.log(`🔍 Found ${matches} occurrences of "End of script"`);
    return matches > 1;
}

function typeAndSubmit() {
    try {
        // Check if we should stop
        if (checkForEndOfScript()) {
            console.log('🛑 "End of script" detected more than once! Stopping auto-submit...');
            stopAutoSubmit();
            return false;
        }

        // Find the text input area (ProseMirror editor)
        const textArea = document.querySelector('#prompt-textarea');

        if (!textArea) {
            console.log('Text area not found');
            return false;
        }

        // Clear existing content and set new content
        textArea.innerHTML = '<p>CONTINUE</p>';

        // Trigger input events to ensure the interface recognizes the change
        textArea.dispatchEvent(new Event('input', { bubbles: true }));
        textArea.dispatchEvent(new Event('change', { bubbles: true }));

        // Small delay to ensure content is processed
        setTimeout(() => {
            // Find and click the submit button
            const submitButton = document.querySelector('#composer-submit-button');

            if (submitButton && !submitButton.disabled) {
                submitButton.click();
                console.log('✅ Submitted "CONTINUE" at', new Date().toLocaleTimeString());
                return true;
            } else {
                console.log('❌ Submit button not found or disabled');
                return false;
            }
        }, 100);

    } catch (error) {
        console.error('Error in typeAndSubmit:', error);
        return false;
    }
}

function startAutoSubmit() {
    console.log('🚀 Starting auto-submit every 2 minutes...');
    console.log('📍 Will stop automatically when "End of script" is detected in DOM');

    // Check immediately if we should even start
    if (checkForEndOfScript()) {
        console.log('🛑 "End of script" already detected more than once! Not starting auto-submit.');
        return;
    }

    // Submit immediately first
    typeAndSubmit();

    // Then set up interval for every 2 minutes
    autoSubmitInterval = setInterval(() => {
        typeAndSubmit();
    }, 120000); // 2 minutes = 120000 milliseconds

    console.log('✅ Auto-submit started. Will check for "End of script" every 2 minutes.');
}

function stopAutoSubmit() {
    if (autoSubmitInterval) {
        clearInterval(autoSubmitInterval);
        autoSubmitInterval = null;
        console.log('🛑 Auto-submit stopped.');
    } else {
        console.log('No auto-submit running.');
    }
}

// Alternative method using contenteditable manipulation
function typeAndSubmitAlternative() {
    try {
        // Check if we should stop
        if (checkForEndOfScript()) {
            console.log('🛑 "End of script" detected more than once! Not submitting...');
            return false;
        }

        const textArea = document.querySelector('#prompt-textarea');

        if (!textArea) {
            console.log('Text area not found');
            return false;
        }

        // Focus the text area
        textArea.focus();

        // Clear content
        textArea.innerHTML = '';

        // Insert text
        const textNode = document.createTextNode('CONTINUE');
        const paragraph = document.createElement('p');
        paragraph.appendChild(textNode);
        textArea.appendChild(paragraph);

        // Trigger events
        const inputEvent = new InputEvent('input', {
            bubbles: true,
            cancelable: true,
            inputType: 'insertText',
            data: 'CONTINUE'
        });
        textArea.dispatchEvent(inputEvent);

        // Submit after a short delay
        setTimeout(() => {
            const submitButton = document.querySelector('#composer-submit-button');
            if (submitButton && !submitButton.disabled) {
                submitButton.click();
                console.log('✅ Alternative method: Submitted "CONTINUE" at', new Date().toLocaleTimeString());
            }
        }, 200);

    } catch (error) {
        console.error('Error in alternative method:', error);
    }
}

// Manual check function
function checkEndStatus() {
    const hasEndText = checkForEndOfScript();
    console.log(`🔍 End of script status: ${hasEndText ? 'FOUND' : 'NOT FOUND'}`);
    return hasEndText;
}

// Start the auto-submit
startAutoSubmit();

// Export functions to global scope for manual control
window.startAutoSubmit = startAutoSubmit;
window.stopAutoSubmit = stopAutoSubmit;
window.typeAndSubmitAlternative = typeAndSubmitAlternative;
window.checkEndStatus = checkEndStatus;

console.log('📝 Auto-continue script loaded!');
console.log('Commands available:');
console.log('  - startAutoSubmit() - Start auto-submitting every 2 minutes');
console.log('  - stopAutoSubmit() - Stop auto-submitting');
console.log('  - typeAndSubmitAlternative() - Try alternative method');
console.log('  - checkEndStatus() - Manually check if "End of script" is in DOM');
console.log('⏰ Interval: 2 minutes (120 seconds)');
console.log('🎯 Stop condition: "End of script" text found in DOM');