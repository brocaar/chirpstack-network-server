var loraserver = angular.module('loraserver', [
    'ngRoute',

    'loraserverControllers'
    ]);

loraserver.config(["$provide", function($provide) {
    return $provide.decorator('$http', ['$delegate', function($delegate) {
        $delegate.rpc = function(method, parameters) {
            var data = {"method": method, "params": [parameters], "id" : 1};
            return $delegate.post('/rpc', data, {'headers':{'Content-Type': 'application/json'}});
        };
        return $delegate;
        }]);
    }]);

loraserver.config(['$routeProvider',
    function($routeProvider) {
        $routeProvider.
            when('/applications', {
                templateUrl: 'partials/applications.html',
                controller: 'ApplicationListCtrl'
            }).
            when('/applications/:application', {
                templateUrl: 'partials/applications.html',
                controller: 'ApplicationListCtrl'
            }).
            when('/nodes', {
                templateUrl: 'partials/nodes.html',
                controller: 'NodeListCtrl'
            }).
            when('/nodes/:node', {
                templateUrl: 'partials/nodes.html',
                controller: 'NodeListCtrl'
            }).
            otherwise({
                redirectTo: '/applications'
            });
        }]);

var loraserverControllers = angular.module('loraserverControllers', []);

// manage applications
loraserverControllers.controller('ApplicationListCtrl', ['$scope', '$http', '$routeParams', '$route',
    function ($scope, $http, $routeParams, $route) {
        $scope.page = 'applications';
        $http.rpc('API.GetApplications', {'limit': 9999, 'offset': 0}).success(function(data) {
                $scope.apps = data.result;
        });

        $scope.create = function(app) {
            if(app == null) {
                $('#createModal').modal().on('hidden.bs.modal', function() {
                    $route.reload();
                });
            } else {
                $http.rpc('API.CreateApplication', app).success(function(data) {
                    if (data.error == null) {
                        $('#createModal').modal('hide');
                    }
                    $scope.error = data.error;
                });
            }
        };

        $scope.update = function(app) {
            $http.rpc('API.UpdateApplication', app).success(function(data) {
                if (data.error == null) {
                    $('#editModal').modal('hide');
                }
                $scope.error = data.error;
            });
        };

        $scope.delete = function(app) {
            if (confirm('Are you sure you want to delete ' + app.appEUI + '?')) {
                $http.rpc('API.DeleteApplication', app.appEUI).success(function(data) {
                    if (data.error != null) {
                        alert(data.error);
                    }
                    $route.reload();
                });
            }
        };

        if($routeParams.application) {
            $http.rpc('API.GetApplication', $routeParams.application).success(function(data) {
                $scope.app = data.result;
                $('#editModal').modal().on('hidden.bs.modal', function() { history.go(-1); });
            });
        };
    }]);

// manage nodes
loraserverControllers.controller('NodeListCtrl', ['$scope', '$http', '$routeParams', '$route',
    function ($scope, $http, $routeParams, $route) {
        $http.rpc('API.GetNodes', {'limit': 9999, 'offset': 0}).success(function(data) {
            $scope.nodes = data.result;
        });

        $scope.create = function(node) {
            if (node == null) {
                $('#createModal').modal().on('hidden.bs.modal', function(){
                    $route.reload();
                });
            } else {
                $http.rpc('API.CreateNode', node).success(function(data) {
                    if (data.error == null) {
                        $('#createModal').modal('hide');
                    }
                    $scope.error = data.error;
                });
            }
        };

        $scope.update = function(node) {
            $http.rpc('API.UpdateNode', node).success(function(data) {
                if (data.error == null) {
                    $('#editModal').modal('hide');
                }
                $scope.error = data.error;
            });
        };

        $scope.delete = function(node) {
            if (confirm('Are you sure you want to delete ' + node.devEUI + '?')) {
                $http.rpc('API.DeleteNode', node.devEUI).success(function(data) {
                   if (data.error != null) {
                        alert(data.error);
                    }
                    $route.reload();
                });
            }
        };

        $scope.session = function(node) {
            $http.rpc('API.GetNodeSessionByDevEUI', node.devEUI).success(function(data) {
                $scope.ns = data.result;
                if ($scope.ns == null) {
                    $scope.ns = {
                        devEUI: node.devEUI,
                        appEUI: node.appEUI,
                        fCntUp: 0,
                        fCntDown: 0
                    };
                }
                $('#sessionModal').modal().on('hidden.bs.modal', function() {
                    $route.reload();
                });
            });
        };

        $scope.updateSession = function(ns) {
            $http.rpc('API.UpdateNodeSession', ns).success(function(data) {
               if (data.error == null) {
                    $('#sessionModal').modal('hide');
                }
                $scope.error = data.error; 
            });
        };

        $scope.getRandomDevAddr = function(ns) {
            $http.rpc('API.GetRandomDevAddr', null).success(function(data) {
                ns.devAddr = data.result;
                $scope.error = data.error;
            });
        };

        if($routeParams.node) {
            $http.rpc('API.GetNode', $routeParams.node).success(function(data) {
                $scope.node = data.result;
                console.log(data);
                $('#editModal').modal().on('hidden.bs.modal', function() { history.go(-1); });
            });
        };
        $scope.page = 'nodes';
    }]);