var directives = directives || {};

directives.patch = angular.module('directives.patch', ['filters.common', 'directives.github']);

directives.patch.directive('patchCommitPanel', function() {
  return {
    scope : true,
    restrict : 'E',
    templateUrl : '/static/partials/patch_commit_panel.html',
    link : function(scope, element, attrs) {
      scope.patchinfo = {};
      scope.basecommit = {};
      scope.$parent.$watch(attrs.basecommit, function(v) {
        scope.basecommit = v;
      });
      scope.$parent.$watch(attrs.patchinfo, function(v) {
        scope.patchinfo = v;
        // get diff totals
        totalAdd = 0;
        totalDel = 0;
        _.each(scope.patchinfo.Patch.Patches, function(patch) {
          _.each(patch.PatchSet.Summary, function(diff) {
            totalAdd += diff.Additions;
            totalDel += diff.Deletions;
          })
        });
        scope.totals = {additions: totalAdd, deletions: totalDel};
      });
      scope.timezone = attrs.timezone;
      scope.base = attrs.base;
      scope.baselink = attrs.baselink;
    }
  };
});

directives.patch.directive('patchDiffPanel', function() {
  return {
    scope : true,
    restrict : 'E',
    templateUrl : '/static/partials/patch_diff.html',
    link : function(scope, element, attrs) {

      scope.baselink = attrs.baselink;
      scope.type = attrs.type;

      // lookup table with constants for sorting / displaying diff results.
      // there is redundancy here between "success/pass" and "failed/fail"
      // to allow this to work generically with both test and task statuses
      scope.diffTypes = {
        successfailed:  {icon:"icon-bug", type: 0},
        passfail:       {icon:"icon-bug", type: 0},
        failedfailed:   {icon:"icon-question", type: 1},
        failfail:       {icon:"icon-question", type: 1},
        failedsuccess:  {icon:"icon-star", type: 2},
        failpass:       {icon:"icon-star", type: 2},
        successsuccess: {icon:"", type: 3},
        passpass:       {icon:"", type: 3},
      };

      // helper for ranking status combinations
      scope.getDiffStatus = function(statusDiff) {
        // concat results for key lookup
        key = statusDiff.diff.original + statusDiff.diff.patch;
        if (key in scope.diffTypes) {
            return scope.diffTypes[key];
        }
        // else return a default
        return {icon: "", type:1000};
      }

      scope.diffs = [];
      scope.$parent.$watch(attrs.diffs, function(d) {
        // only iterate if valid diffs are given
        if (!d || !d.length) {
          return
        }
        scope.diffs = d;
        _.each(scope.diffs, function(diff) {
          diff.originalLink = scope.baselink + diff.original;
          diff.patchLink = scope.baselink + diff.patch;
          diff.displayInfo = scope.getDiffStatus(diff);
        });
      });

    }
  };
});
