// Namespace.
var pushgateway = {};

pushgateway.labels = {};
pushgateway.panel = null;

pushgateway.switchToMetrics = function(){
    $('#metrics-div').show();
    $('#status-div').hide();
    $('#metrics-li').addClass('active');
    $('#status-li').removeClass('active');
}

pushgateway.switchToStatus = function(){
    $('#metrics-div').hide();
    $('#status-div').show();
    $('#metrics-li').removeClass('active');
    $('#status-li').addClass('active');
}

pushgateway.showDelModal = function(labels, labelsEncoded, panelID, event){
    event.stopPropagation(); // Don't trigger accordion collapse.
    pushgateway.labels = labelsEncoded;
    pushgateway.panel = $('#' + panelID).parent();

    var components = [];
    for (var ln in labels) {
	components.push(ln + '="' + labels[ln] + '"')
    }
    
    $('#del-modal-msg').text(
	'Do you really want to delete all metrics of group {' + components.join(', ') + '}?'
    );
    $('#del-modal').modal('show');
}

pushgateway.showDelAllModal = function(){
    if (!$('button#del-all').hasClass('disabled')) {
        $('#del-modal-all-msg').text(
            'Do you really want to delete all metrics from all metric groups?'
        );
        $('#del-all-modal').modal('show');
    }
}

pushgateway.deleteGroup = function(){
    var pathElements = [];
    for (var ln in pushgateway.labels) {
	if (ln != 'job') {
	    pathElements.push(encodeURIComponent(ln+'@base64'));
	    pathElements.push(encodeURIComponent(pushgateway.labels[ln]));
	}
    }
    var groupPath = pathElements.join('/');
    if (groupPath.length > 0) {
	groupPath = '/' + groupPath;
    }
    
    $.ajax({
	type: 'DELETE',
	url: 'metrics/job@base64/' + encodeURIComponent(pushgateway.labels['job']) + groupPath,
	success: function(data, textStatus, jqXHR) {
	    pushgateway.panel.remove();
        pushgateway.decreaseDelAllCounter();
	    $('#del-modal').modal('hide');
	},
	error: function(jqXHR, textStatus, error) {
	    alert('Deleting metric group failed: ' + error);
	}
    });
}

pushgateway.deleteAllGroup = function(){
    $.ajax({
        type: 'PUT',
        url: 'api/v1/admin/wipe',
        success: function(data, textStatus, jqXHR) {
            $('div').each(function() {
                id = $(this).attr("id");
                if (typeof id != 'undefined' && id.match(/^group-panel-[0-9]{1,}$/)) {
                    $(this).parent().remove();
                }
            });
            pushgateway.setDelAllCounter(0);
            $('#del-all-modal').modal('hide');
        },
        error: function(jqXHR, textStatus, error) {
            alert('Deleting all metric groups failed: ' + error);
        }
    });
}

pushgateway.decreaseDelAllCounter = function(){
    var counter = parseInt($('span#del-all-counter').text());
    pushgateway.setDelAllCounter(--counter);
}

pushgateway.setDelAllCounter = function(n){
    $('span#del-all-counter').text(n);
    if (n <= 0) {
        pushgateway.disableDelAllGroupButton();
        return;
    }
    pushgateway.enableDelAllGroupButton();
}

pushgateway.enableDelAllGroupButton = function(){
    $('button#del-all').removeClass('disabled');
}

pushgateway.disableDelAllGroupButton = function(){
    $('button#del-all').addClass('disabled');
}

$(function () {
    $('div.collapse').on('show.bs.collapse', function (event) {
	$(this).prev().find('span.toggle-icon')
	    .removeClass('glyphicon-collapse-down')
	    .addClass('glyphicon-collapse-up');
	event.stopPropagation();
    })
    $('div.collapse').on('hide.bs.collapse', function (event) {
	$(this).prev().find('span.toggle-icon')
	    .removeClass('glyphicon-collapse-up')
	    .addClass('glyphicon-collapse-down');
	event.stopPropagation();
    })
})
