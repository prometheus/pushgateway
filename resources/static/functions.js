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

pushgateway.showDelModal = function(labels, panelID, event){
    event.stopPropagation(); // Don't trigger accordion collapse.
    pushgateway.labels = labels;
    pushgateway.panel = $('#' + panelID);

    var components = [];
    for (var ln in labels) {
	components.push(ln + '="' + labels[ln] + '"')
    }
    
    $('#del-modal-msg').text(
	'Do you really want to delete all metrics of group {' + components.join(', ') + '}?'
    );
    $('#del-modal').modal('show');
}

pushgateway.deleteGroup = function(){
    var pathElements = [];
    for (var ln in pushgateway.labels) {
	if (ln != 'job') {
	    pathElements.push(encodeURIComponent(ln));
	    pathElements.push(encodeURIComponent(pushgateway.labels[ln]));
	}
    }
    var groupPath = pathElements.join('/');
    if (groupPath.length > 0) {
	groupPath = '/' + groupPath;
    }
    
    $.ajax({
	type: 'DELETE',
	url: 'metrics/job/' + encodeURIComponent(pushgateway.labels['job']) + groupPath,
	success: function(data, textStatus, jqXHR) {
	    pushgateway.panel.remove();
	    $('#del-modal').modal('hide');
	},
	error: function(jqXHR, textStatus, error) {
	    alert('Deleting metric group failed: ' + error);
	}
    });
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
