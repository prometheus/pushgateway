// Namespace.
var pushgateway = {};

pushgateway.jobName = '';
pushgateway.jobPanel = null;
pushgateway.instanceName = '';
pushgateway.instancePanel = null;

pushgateway.switchToMetrics = function(){
    $( '#metrics-div' ).removeClass( 'hidden' );
    $( '#status-div' ).addClass( 'hidden' );
    $( '#metrics-li' ).addClass( 'active' );
    $( '#status-li' ).removeClass( 'active' );
}

pushgateway.switchToStatus = function(){
    $( '#metrics-div' ).addClass( 'hidden' );
    $( '#status-div' ).removeClass( 'hidden' );
    $( '#metrics-li' ).removeClass( 'active' );
    $( '#status-li' ).addClass( 'active' );
}

pushgateway.showJobModal = function( jobName, jobPanelID, event ){
    event.stopPropagation(); // Don't trigger accordion collapse.
    pushgateway.jobName = jobName;
    pushgateway.jobPanel = $( '#' + jobPanelID );
    $( '#del-job-modal-msg' ).text(
	'Do you really want to delete all metrics of job="' + jobName + '"?'
    );
    $( '#del-job-modal' ).modal( 'show' );
}

pushgateway.showInstanceModal = function( jobName, instanceName, instancePanelID, event ){
    event.stopPropagation(); // Don't trigger accordion collapse.
    pushgateway.jobName = jobName;
    pushgateway.instanceName = instanceName;
    pushgateway.instancePanel = $( '#' + instancePanelID );
    $( '#del-instance-modal-msg' ).text(
	'Do you really want to delete all metrics of job="' + jobName +
	    '", instance="' + instanceName + '"?'
    );
    $( '#del-instance-modal' ).modal( 'show' );
}

pushgateway.deleteJob = function(){
    $.ajax({
	type: 'DELETE',
	url: '/metrics/jobs/' + pushgateway.jobName,
	success: function( data, textStatus, jqXHR ) {
	    pushgateway.jobPanel.remove();
	    $( '#del-job-modal' ).modal( 'hide' );
	},
	error: function(jqXHR, textStatus, error) {
	    alert( 'Deleting job failed: ' + error );
	}
    });
}

pushgateway.deleteInstance = function(){
    $.ajax({
	type: 'DELETE',
	url: '/metrics/jobs/' + escape( pushgateway.jobName ) +
	    '/instances/' + escape( pushgateway.instanceName ),
	success: function( data, textStatus, jqXHR ) {
	    pushgateway.instancePanel.remove();
	    $( '#del-instance-modal' ).modal( 'hide' );
	},
	error: function(jqXHR, textStatus, error) {
	    alert( 'Deleting instance failed: ' + error );
	}
    });
}
